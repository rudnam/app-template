package server_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/francis/app-template/internal/config"
)

// fakeGoogle is Google's token endpoint: it answers any code with an ID
// token carrying claims, after checking what the app sent.
type fakeGoogle struct {
	t      *testing.T
	claims map[string]any
}

func (g *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if r.Form.Get("client_secret") != "secret" || r.Form.Get("code_verifier") == "" || r.Form.Get("code") != "the-code" {
		g.t.Errorf("token request: %v", r.Form)
		w.WriteHeader(400)
		return
	}
	payload, _ := json.Marshal(g.claims)
	token := "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
	json.NewEncoder(w).Encode(map[string]string{"id_token": token})
}

// newGoogleServer starts the app with Sign in with Google pointed at a fake.
func newGoogleServer(t *testing.T, edit ...func(*config.Config)) (*httptest.Server, *fakeGoogle) {
	g := &fakeGoogle{t: t}
	tokens := httptest.NewServer(g)
	t.Cleanup(tokens.Close)
	ts := newTestServer(t, append([]func(*config.Config){func(c *config.Config) {
		c.GoogleClientID = "client-id"
		c.GoogleClientSecret = "secret"
		c.PublicOrigin = "https://app.example"
		c.GoogleAuthURL = "https://google.example/auth"
		c.GoogleTokenURL = tokens.URL
	}}, edit...)...)
	return ts, g
}

// googleSignIn walks the browser through the redirects, playing the
// person who picks their Google account, and returns where the app lands
// them afterwards.
func googleSignIn(t *testing.T, c *client, g *fakeGoogle, sub, email string) string {
	t.Helper()
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { c.http.CheckRedirect = nil }()

	resp, err := c.http.Get(c.base + "/api/auth/google")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	to, _ := url.Parse(resp.Header.Get("Location"))
	q := to.Query()
	if resp.StatusCode != 302 || to.Host != "google.example" || q.Get("client_id") != "client-id" ||
		q.Get("redirect_uri") != "https://app.example/api/auth/google/callback" || q.Get("scope") != "openid email" {
		t.Fatalf("start: %d %s", resp.StatusCode, to)
	}
	g.claims = map[string]any{
		"iss": "https://accounts.google.com", "aud": "client-id", "exp": time.Now().Add(time.Hour).Unix(),
		"sub": sub, "email": email, "email_verified": true, "nonce": q.Get("nonce"),
	}
	resp, err = c.http.Get(c.base + "/api/auth/google/callback?code=the-code&state=" + url.QueryEscape(q.Get("state")))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 302 {
		t.Fatalf("callback: %d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

func TestGoogleSignIn(t *testing.T) {
	ts, g := newGoogleServer(t)

	var opts struct{ Google bool }
	if code := newClient(t, ts).do("GET", "/api/auth/options", nil, &opts); code != 200 || !opts.Google {
		t.Fatalf("options: %d %+v", code, opts)
	}

	// A new person gets an account.
	c := newClient(t, ts)
	if to := googleSignIn(t, c, g, "sub-1", "Grace@Gmail.com"); to != "/app/" {
		t.Fatalf("first sign-in landed on %q", to)
	}
	var me struct{ Email string }
	if code := c.do("GET", "/api/me", nil, &me); code != 200 || me.Email != "grace@gmail.com" {
		t.Fatalf("me: %d %q", code, me.Email)
	}
	// The account has no password to guess.
	if code := newClient(t, ts).do("POST", "/api/login", map[string]string{"email": "grace@gmail.com", "password": ""}, nil); code != 401 {
		t.Errorf("password sign-in to a Google account: %d, want 401", code)
	}

	// Next time it's the same account, even if the Gmail address changed.
	again := newClient(t, ts)
	googleSignIn(t, again, g, "sub-1", "grace.h@gmail.com")
	if again.do("GET", "/api/me", nil, &me); me.Email != "grace@gmail.com" {
		t.Errorf("second sign-in got %q", me.Email)
	}

	// Someone who signed up with a password and confirmed their email can
	// use Google with that email too, and keeps the password.
	f := &fakeMail{}
	ts, g = newGoogleServer(t, withMail(f))
	pw := newClient(t, ts)
	pw.signup("ada@gmail.com")
	pw.open(f.lastLink(t, "ada@gmail.com"))
	viaGoogle := newClient(t, ts)
	if to := googleSignIn(t, viaGoogle, g, "sub-2", "ada@gmail.com"); to != "/app/" {
		t.Fatalf("linking landed on %q", to)
	}
	if viaGoogle.do("GET", "/api/me", nil, &me); me.Email != "ada@gmail.com" {
		t.Errorf("linked sign-in got %q", me.Email)
	}
	if code := newClient(t, ts).do("POST", "/api/login", map[string]string{"email": "ada@gmail.com", "password": password}, nil); code != 200 {
		t.Errorf("password after linking: %d", code)
	}
	// ...but a different Google account with that email can't take it over.
	if to := googleSignIn(t, newClient(t, ts), g, "sub-3", "ada@gmail.com"); to != "/app/?signin_error=linked" {
		t.Errorf("second Google account landed on %q", to)
	}

	// Someone who signed up with another person's email and never
	// confirmed it loses the account when its owner signs in with Google.
	squatter := newClient(t, ts)
	squatter.signup("victim@gmail.com")
	owner := newClient(t, ts)
	googleSignIn(t, owner, g, "sub-4", "victim@gmail.com")
	if code := squatter.do("GET", "/api/me", nil, nil); code != 401 {
		t.Errorf("squatter's session after the owner signed in: %d, want 401", code)
	}
	if code := newClient(t, ts).do("POST", "/api/login", map[string]string{"email": "victim@gmail.com", "password": password}, nil); code != 401 {
		t.Errorf("squatter's password after the owner signed in: %d, want 401", code)
	}
	var mine struct {
		Email        string
		HasPassword  bool `json:"has_password"`
		ConfirmEmail bool `json:"confirm_email"`
	}
	if owner.do("GET", "/api/me", nil, &mine); mine.Email != "victim@gmail.com" || mine.HasPassword || mine.ConfirmEmail {
		t.Errorf("owner: %+v", mine)
	}

	// A Google-only account is deleted by typing its email.
	if code := owner.do("DELETE", "/api/me", map[string]string{"email": "someone@gmail.com"}, nil); code != 400 {
		t.Errorf("delete with the wrong email: %d, want 400", code)
	}
	if code := owner.do("DELETE", "/api/me", map[string]string{"email": "Victim@gmail.com"}, nil); code != 204 {
		t.Errorf("delete: %d", code)
	}
}

func TestGoogleSignInRefusals(t *testing.T) {
	ts, g := newGoogleServer(t, func(c *config.Config) { c.AllowSignup = false })
	c := newClient(t, ts)
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	callback := func(query string) string {
		resp, err := c.http.Get(ts.URL + "/api/auth/google/callback?" + query)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.Header.Get("Location")
	}
	// Without the cookie from the start, a callback is refused.
	if to := callback("code=the-code&state=guess"); to != "/app/?signin_error=expired" {
		t.Errorf("forged callback landed on %q", to)
	}
	if to := callback("error=access_denied"); to != "/app/?signin_error=cancelled" {
		t.Errorf("cancel landed on %q", to)
	}
	// Sign-up is closed, so a new Google account gets no account.
	c.http.CheckRedirect = nil
	if to := googleSignIn(t, c, g, "sub-9", "new@gmail.com"); to != "/app/?signin_error=closed" {
		t.Errorf("closed sign-up landed on %q", to)
	}
	if code := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Errorf("me after refused sign-in: %d", code)
	}
}

func TestGoogleOffByDefault(t *testing.T) {
	ts := newTestServer(t)
	var opts struct{ Google bool }
	if newClient(t, ts).do("GET", "/api/auth/options", nil, &opts); opts.Google {
		t.Error("options say Google is on")
	}
	if resp, _ := get(t, ts.URL+"/api/auth/google"); resp.StatusCode != 404 {
		t.Errorf("start without config: %d, want 404", resp.StatusCode)
	}
}
