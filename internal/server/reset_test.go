package server_test

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

var resetLinkRe = regexp.MustCompile(`https://app\.example/app/#/reset\?token=([\w-]+)`)

// resetTokens returns the tokens of every reset link sent to "to", oldest first.
func (f *fakeMail) resetTokens(to string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var tokens []string
	for _, m := range f.sent {
		if m.To == to {
			if got := resetLinkRe.FindStringSubmatch(m.Body); got != nil {
				tokens = append(tokens, got[1])
			}
		}
	}
	return tokens
}

// waitResetToken waits for the n-th reset link to "to" (the email is sent in
// the background) and returns its token.
func (f *fakeMail) waitResetToken(t *testing.T, to string, n int) string {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if tokens := f.resetTokens(to); len(tokens) >= n {
			return tokens[n-1]
		}
	}
	t.Fatalf("no reset link %d sent to %s", n, to)
	return ""
}

func TestPasswordReset(t *testing.T) {
	f := &fakeMail{}
	ts := newTestServer(t, withMail(f))
	ada := newClient(t, ts)
	ada.signup("ada@example.com")
	// Signed in on a second device too.
	phone := newClient(t, ts)
	if code := phone.do("POST", "/api/login", map[string]string{"email": "ada@example.com", "password": password}, nil); code != 200 {
		t.Fatalf("login: %d", code)
	}

	var opts struct{ Reset bool }
	if newClient(t, ts).do("GET", "/api/auth/options", nil, &opts); !opts.Reset {
		t.Error("reset not offered though the server sends email")
	}

	// Asking answers the same whether or not the account exists.
	visitor := newClient(t, ts)
	for _, email := range []string{"nobody@example.com", "ADA@example.com "} {
		if code := visitor.do("POST", "/api/password/forgot", map[string]string{"email": email}, nil); code != 204 {
			t.Fatalf("forgot %q: %d", email, code)
		}
	}
	token := f.waitResetToken(t, "ada@example.com", 1)
	if code := visitor.do("POST", "/api/password/forgot", map[string]string{"email": "not an email"}, nil); code != 400 {
		t.Errorf("forgot with a bad address: %d", code)
	}
	// Asking again straight away sends nothing more.
	visitor.do("POST", "/api/password/forgot", map[string]string{"email": "ada@example.com"}, nil)
	time.Sleep(200 * time.Millisecond)
	if n := len(f.resetTokens("ada@example.com")); n != 1 {
		t.Errorf("%d reset links sent within a minute, want 1", n)
	}
	if n := len(f.resetTokens("nobody@example.com")); n != 0 {
		t.Errorf("a reset link went to an address without an account")
	}

	newPassword := "a brand new password"
	if code := visitor.do("POST", "/api/password/reset", map[string]string{"token": "made-up", "password": newPassword}, nil); code != 400 {
		t.Errorf("reset with a made-up token: %d", code)
	}
	if code := visitor.do("POST", "/api/password/reset", map[string]string{"token": token, "password": "short"}, nil); code != 400 {
		t.Errorf("reset to a short password: %d", code)
	}
	var got me
	if code := visitor.do("POST", "/api/password/reset", map[string]string{"token": token, "password": newPassword}, &got); code != 200 {
		t.Fatalf("reset: %d", code)
	}
	if got.Email != "ada@example.com" || !got.HasPassword || got.ConfirmEmail {
		t.Errorf("after reset: %+v, want signed in with the email confirmed", got)
	}
	if code := visitor.do("GET", "/api/me", nil, nil); code != 200 {
		t.Errorf("not signed in after reset: %d", code)
	}

	// Every other session ended, and only the new password works.
	for name, c := range map[string]*client{"browser": ada, "phone": phone} {
		if code := c.do("GET", "/api/me", nil, nil); code != 401 {
			t.Errorf("%s still signed in after reset: %d", name, code)
		}
	}
	if code := newClient(t, ts).do("POST", "/api/login", map[string]string{"email": "ada@example.com", "password": password}, nil); code != 401 {
		t.Errorf("old password: %d", code)
	}
	if code := newClient(t, ts).do("POST", "/api/login", map[string]string{"email": "ada@example.com", "password": newPassword}, nil); code != 200 {
		t.Errorf("new password: %d", code)
	}

	// The link works once.
	var e struct{ Error string }
	if code := newClient(t, ts).do("POST", "/api/password/reset", map[string]string{"token": token, "password": "yet another password"}, &e); code != 400 || !strings.Contains(e.Error, "expired") {
		t.Errorf("used link: %d %q", code, e.Error)
	}
}

func TestPasswordResetOnlyNewestLinkWorks(t *testing.T) {
	f := &fakeMail{}
	ts, conn := newTestServerDB(t, withMail(f))
	newClient(t, ts).signup("ada@example.com")
	c := newClient(t, ts)
	c.do("POST", "/api/password/forgot", map[string]string{"email": "ada@example.com"}, nil)
	first := f.waitResetToken(t, "ada@example.com", 1)
	// Pretend the first link is older than the gap between links.
	if _, err := conn.Exec(`UPDATE password_tokens SET created_at = now() - interval '2 minutes'`); err != nil {
		t.Fatal(err)
	}
	c.do("POST", "/api/password/forgot", map[string]string{"email": "ada@example.com"}, nil)
	second := f.waitResetToken(t, "ada@example.com", 2)
	if code := c.do("POST", "/api/password/reset", map[string]string{"token": first, "password": "a brand new password"}, nil); code != 400 {
		t.Errorf("older link: %d, want 400", code)
	}
	if code := c.do("POST", "/api/password/reset", map[string]string{"token": second, "password": "a brand new password"}, nil); code != 200 {
		t.Errorf("newest link: %d", code)
	}
}

func TestNoPasswordResetWithoutMail(t *testing.T) {
	ts := newTestServer(t)
	c := newClient(t, ts)
	var opts struct{ Reset bool }
	if c.do("GET", "/api/auth/options", nil, &opts); opts.Reset {
		t.Error("reset offered though the server can't send email")
	}
	if code := c.do("POST", "/api/password/forgot", map[string]string{"email": "ada@example.com"}, nil); code != 404 {
		t.Errorf("forgot: %d, want 404", code)
	}
}
