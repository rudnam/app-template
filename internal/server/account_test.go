package server_test

import (
	"testing"

	"github.com/francis/app-template/internal/config"
)

func TestAuthFlow(t *testing.T) {
	ts := newTestServer(t)
	c := newClient(t, ts)

	if code := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("me before signup: %d, want 401", code)
	}
	if code := c.do("POST", "/api/signup", map[string]string{"email": "a@example.com", "password": "short"}, nil); code != 400 {
		t.Errorf("short password: %d, want 400", code)
	}
	if code := c.do("POST", "/api/signup", map[string]string{"email": "not-an-email", "password": password}, nil); code != 400 {
		t.Errorf("bad email: %d, want 400", code)
	}
	c.signup("Ada@Example.com")

	var me struct{ Email string }
	if code := c.do("GET", "/api/me", nil, &me); code != 200 || me.Email != "ada@example.com" {
		t.Fatalf("me: %d %q, want 200 and the lower-cased email", code, me.Email)
	}
	if code := c.do("POST", "/api/signup", map[string]string{"email": "ada@example.com", "password": password}, nil); code != 409 {
		t.Errorf("duplicate signup: %d, want 409", code)
	}
	if code := c.do("POST", "/api/logout", nil, nil); code != 204 {
		t.Fatalf("logout: %d", code)
	}
	if code := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Fatalf("me after logout: %d, want 401", code)
	}

	other := newClient(t, ts)
	if code := other.do("POST", "/api/login", map[string]string{"email": "ada@example.com", "password": "wrong password!"}, nil); code != 401 {
		t.Errorf("wrong password: %d, want 401", code)
	}
	if code := other.do("POST", "/api/login", map[string]string{"email": "nobody@example.com", "password": password}, nil); code != 401 {
		t.Errorf("unknown email: %d, want 401", code)
	}
	if code := c.do("POST", "/api/login", map[string]string{"email": "ADA@example.com", "password": password}, nil); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if code := c.do("GET", "/api/me", nil, nil); code != 200 {
		t.Fatalf("me after login: %d", code)
	}
}

func TestSignupCanBeClosed(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.AllowSignup = false })
	if code := newClient(t, ts).do("POST", "/api/signup", map[string]string{"email": "a@example.com", "password": password}, nil); code != 403 {
		t.Fatalf("signup while closed: %d, want 403", code)
	}
}

func TestAuthIsRateLimited(t *testing.T) {
	ts := newTestServer(t, func(c *config.Config) { c.AuthRateLimit = 2 })
	c := newClient(t, ts)
	bad := map[string]string{"email": "a@example.com", "password": "wrong password!"}
	for i, want := range []int{401, 401, 429} {
		if code := c.do("POST", "/api/login", bad, nil); code != want {
			t.Fatalf("attempt %d: %d, want %d", i+1, code, want)
		}
	}
}
