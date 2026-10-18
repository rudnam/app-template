package server_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/francis/app-template/internal/config"
	"github.com/francis/app-template/internal/mailer"
)

// fakeMail keeps what the app sends instead of sending it.
type fakeMail struct {
	mu   sync.Mutex
	sent []mailer.Message
}

func (f *fakeMail) Send(_ context.Context, m mailer.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

var linkRe = regexp.MustCompile(`https://app\.example(/api/verify-email\?token=[\w-]+)`)

// lastLink returns the path of the newest confirmation link sent to "to".
func (f *fakeMail) lastLink(t *testing.T, to string) string {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.sent) - 1; i >= 0; i-- {
		if f.sent[i].To == to {
			if m := linkRe.FindStringSubmatch(f.sent[i].Body); m != nil {
				return m[1]
			}
		}
	}
	t.Fatalf("no link sent to %s in %+v", to, f.sent)
	return ""
}

func withMail(f *fakeMail) func(*config.Config) {
	return func(c *config.Config) {
		c.Mailer = f
		c.PublicOrigin = "https://app.example"
	}
}

// open follows a link without following redirects and returns where it sends the browser.
func (c *client) open(path string) string {
	c.t.Helper()
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { c.http.CheckRedirect = nil }()
	resp, err := c.http.Get(c.base + path)
	if err != nil {
		c.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.Header.Get("Location")
}

type me struct {
	Email        string
	HasPassword  bool `json:"has_password"`
	ConfirmEmail bool `json:"confirm_email"`
}

func TestEmailConfirmation(t *testing.T) {
	f := &fakeMail{}
	ts := newTestServer(t, withMail(f))
	c := newClient(t, ts)
	c.signup("ada@example.com")

	var got me
	if c.do("GET", "/api/me", nil, &got); !got.ConfirmEmail || !got.HasPassword {
		t.Fatalf("after signup: %+v, want a reminder to confirm", got)
	}
	first := f.lastLink(t, "ada@example.com")
	if !strings.Contains(f.sent[0].Subject, "Confirm") {
		t.Errorf("subject %q", f.sent[0].Subject)
	}

	// Asking again sends a new link, and only the newest one works.
	if code := c.do("POST", "/api/verify-email", nil, nil); code != 204 {
		t.Fatalf("resend: %d", code)
	}
	second := f.lastLink(t, "ada@example.com")
	if second == first {
		t.Fatal("resend sent the same link")
	}
	// The link works from any browser, signed in or not.
	if to := newClient(t, ts).open(first); to != "/app/?email=expired" {
		t.Errorf("old link went to %q", to)
	}
	if to := newClient(t, ts).open(second); to != "/app/?email=confirmed" {
		t.Errorf("new link went to %q", to)
	}
	if to := newClient(t, ts).open(second); to != "/app/?email=expired" {
		t.Errorf("used link went to %q", to)
	}
	if c.do("GET", "/api/me", nil, &got); got.ConfirmEmail {
		t.Errorf("after confirming: %+v", got)
	}
	if code := c.do("POST", "/api/verify-email", nil, nil); code != 409 {
		t.Errorf("resend when confirmed: %d, want 409", code)
	}
}

func TestNoReminderWithoutMail(t *testing.T) {
	ts := newTestServer(t)
	c := newClient(t, ts)
	c.signup("ada@example.com")
	var got me
	if c.do("GET", "/api/me", nil, &got); got.ConfirmEmail {
		t.Errorf("reminder shown though the server can't send email: %+v", got)
	}
	if code := c.do("POST", "/api/verify-email", nil, nil); code != 404 {
		t.Errorf("resend: %d, want 404", code)
	}
}

func TestDeleteAccount(t *testing.T) {
	ts := newTestServer(t)
	c := newClient(t, ts)
	c.signup("ada@example.com")
	if code := c.do("POST", "/api/notes", map[string]string{"body": "Bank PIN hint"}, nil); code != 201 {
		t.Fatalf("create note: %d", code)
	}
	other := newClient(t, ts)
	other.signup("bob@example.com")

	if code := c.do("DELETE", "/api/me", map[string]string{"password": "wrong password!"}, nil); code != 403 {
		t.Fatalf("delete with wrong password: %d, want 403", code)
	}
	if code := c.do("DELETE", "/api/me", map[string]string{"password": password}, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Errorf("me after delete: %d, want 401", code)
	}
	if code := newClient(t, ts).do("POST", "/api/login", map[string]string{"email": "ada@example.com", "password": password}, nil); code != 401 {
		t.Errorf("sign in after delete: %d, want 401", code)
	}
	// The address is free again, and the new account starts empty.
	c.signup("ada@example.com")
	var notes []struct{ Body string }
	if c.do("GET", "/api/notes", nil, &notes); len(notes) != 0 {
		t.Errorf("the deleted account's data is still there: %+v", notes)
	}
	// Nobody else is affected.
	if code := other.do("GET", "/api/me", nil, nil); code != 200 {
		t.Errorf("other user after delete: %d", code)
	}
}
