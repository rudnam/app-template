package server_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/francis/app-template/internal/notes"
)

func TestNotes(t *testing.T) {
	ts := newTestServer(t)
	alice, bob := newClient(t, ts), newClient(t, ts)
	alice.signup("alice@example.com")
	bob.signup("bob@example.com")

	var first, second notes.Note
	if code := alice.do("POST", "/api/notes", map[string]string{"body": "  buy milk  "}, &first); code != 201 || first.Body != "buy milk" {
		t.Fatalf("create: %d %+v (want trimmed body)", code, first)
	}
	alice.do("POST", "/api/notes", map[string]string{"body": "call mum"}, &second)

	var list []notes.Note
	if code := alice.do("GET", "/api/notes", nil, &list); code != 200 || len(list) != 2 || list[0].ID != second.ID {
		t.Fatalf("list: %d %+v (want newest first)", code, list)
	}
	if bob.do("GET", "/api/notes", nil, &list); len(list) != 0 {
		t.Fatalf("bob sees alice's notes: %+v", list)
	}

	// Bob can't delete Alice's note, and can't tell it exists.
	path := fmt.Sprintf("/api/notes/%d", first.ID)
	if code := bob.do("DELETE", path, nil, nil); code != 404 {
		t.Errorf("bob deletes alice's note: %d, want 404", code)
	}
	if code := alice.do("DELETE", path, nil, nil); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code := alice.do("DELETE", path, nil, nil); code != 404 {
		t.Errorf("delete again: %d, want 404", code)
	}
	if code := alice.do("DELETE", "/api/notes/abc", nil, nil); code != 404 {
		t.Errorf("bad id: %d, want 404", code)
	}
}

func TestNoteValidation(t *testing.T) {
	ts := newTestServer(t)
	c := newClient(t, ts)
	c.signup("a@example.com")
	for _, body := range []string{"", "   ", strings.Repeat("x", 10_001)} {
		if code := c.do("POST", "/api/notes", map[string]string{"body": body}, nil); code != 400 {
			t.Errorf("body of %d chars: %d, want 400", len(body), code)
		}
	}
	if code := c.do("POST", "/api/notes", map[string]any{"body": "x", "pinned": true}, nil); code != 400 {
		t.Errorf("unknown field: %d, want 400", code)
	}
}

func TestNotesNeedSignIn(t *testing.T) {
	ts := newTestServer(t)
	c := newClient(t, ts)
	if code := c.do("GET", "/api/notes", nil, nil); code != 401 {
		t.Errorf("GET: %d, want 401", code)
	}
	if code := c.do("POST", "/api/notes", map[string]string{"body": "x"}, nil); code != 401 {
		t.Errorf("POST: %d, want 401", code)
	}
}

func TestCrossOriginWritesAreRefused(t *testing.T) {
	ts := newTestServer(t)
	c := newClient(t, ts)
	c.signup("a@example.com")
	c.headers["Origin"] = "https://evil.example"
	if code := c.do("POST", "/api/notes", map[string]string{"body": "x"}, nil); code != 403 {
		t.Errorf("cross-origin POST: %d, want 403", code)
	}
}
