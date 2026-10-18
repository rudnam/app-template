package server_test

import (
	"strings"
	"testing"

	"github.com/francis/app-template/internal/config"
)

func withDemo(c *config.Config) {
	c.Demo = true
}

type meJSON struct {
	Email string `json:"email"`
	Demo  bool   `json:"demo"`
}

func startDemo(t *testing.T, c *client, lang string) meJSON {
	t.Helper()
	c.headers["Accept-Language"] = lang
	var me meJSON
	if code := c.do("POST", "/api/demo", map[string]string{"tz": "Asia/Tokyo"}, &me); code != 201 {
		t.Fatalf("POST /api/demo: %d", code)
	}
	if !me.Demo {
		t.Fatalf("not a demo: %+v", me)
	}
	return me
}

// notesOf lists the signed-in person's notes, newest first.
func notesOf(t *testing.T, c *client) []string {
	t.Helper()
	var notes []struct{ Body string }
	if code := c.do("GET", "/api/notes", nil, &notes); code != 200 {
		t.Fatalf("GET /api/notes: %d", code)
	}
	bodies := make([]string, len(notes))
	for i, n := range notes {
		bodies[i] = n.Body
	}
	return bodies
}

func TestDemoInJapanese(t *testing.T) {
	ts := newTestServer(t, withDemo)
	c := newClient(t, ts)
	var opts struct{ Demo bool }
	if c.do("GET", "/api/auth/options", nil, &opts); !opts.Demo {
		t.Fatal("options don't offer the demo")
	}
	startDemo(t, c, "ja")
	notes := notesOf(t, c)
	if len(notes) < 3 || !strings.HasPrefix(notes[len(notes)-1], "デモへようこそ") {
		t.Errorf("notes %q", notes)
	}
}

func TestDemoInEnglish(t *testing.T) {
	ts := newTestServer(t, withDemo)
	c := newClient(t, ts)
	startDemo(t, c, "en-US,en;q=0.9")
	notes := notesOf(t, c)
	if len(notes) < 3 || !strings.HasPrefix(notes[len(notes)-1], "Welcome to the demo") {
		t.Errorf("notes %q", notes)
	}
}

func TestDemoIsDeleted(t *testing.T) {
	ts, conn := newTestServerDB(t, withDemo)
	demos := func() (n int) {
		t.Helper()
		if err := conn.QueryRow(`SELECT count(*) FROM users WHERE demo_expires_at IS NOT NULL`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Ending a demo deletes it.
	c := newClient(t, ts)
	startDemo(t, c, "en")
	if c.do("POST", "/api/logout", nil, nil); demos() != 0 {
		t.Errorf("%d demos left after signing out", demos())
	}

	// One whose day is up goes when the next demo starts.
	startDemo(t, newClient(t, ts), "en")
	if _, err := conn.Exec(`UPDATE users SET demo_expires_at = now() - interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	startDemo(t, newClient(t, ts), "en")
	if n := demos(); n != 1 {
		t.Errorf("%d demos; the expired one should be gone", n)
	}
	var orphans int
	conn.QueryRow(`SELECT count(*) FROM notes WHERE user_id NOT IN (SELECT id FROM users)`).Scan(&orphans)
	if orphans != 0 {
		t.Errorf("%d records outlived their demo", orphans)
	}
}

func TestDemoCanBeOff(t *testing.T) {
	ts := newTestServer(t) // Demo false
	c := newClient(t, ts)
	var opts struct{ Demo bool }
	if c.do("GET", "/api/auth/options", nil, &opts); opts.Demo {
		t.Error("options offer a demo that is off")
	}
	if code := c.do("POST", "/api/demo", map[string]string{}, nil); code != 404 {
		t.Errorf("POST /api/demo when off: %d, want 404", code)
	}
	// Nobody can sign up with a demo's made-up address.
	if code := c.do("POST", "/api/signup", map[string]string{"email": "demo-1@demo.invalid", "password": password}, nil); code != 400 {
		t.Errorf("sign-up as a demo address: %d, want 400", code)
	}
}
