package server_test

// Integration tests: they run the real HTTP handler against a real
// PostgreSQL database (see internal/testdb). `./verify.sh go` runs them;
// a plain `go test ./...` skips them.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/francis/app-template/internal/config"
	"github.com/francis/app-template/internal/server"
	"github.com/francis/app-template/internal/testdb"
)

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// newTestServer starts the app on an empty database. edit, if given, changes
// the config first.
func newTestServer(t *testing.T, edit ...func(*config.Config)) *httptest.Server {
	t.Helper()
	ts, _ := newTestServerDB(t, edit...)
	return ts
}

// newTestServerDB is newTestServer that also gives the test its database.
func newTestServerDB(t *testing.T, edit ...func(*config.Config)) (*httptest.Server, *sql.DB) {
	t.Helper()
	conn := testdb.Open(t)
	cfg := config.Config{AllowSignup: true, AuthRateLimit: 1000}
	for _, e := range edit {
		e(&cfg)
	}
	static := fstest.MapFS{
		"index.html":           {Data: []byte("<!doctype html><title>App</title>")},
		"landing.html":         {Data: []byte("<!doctype html><title>Landing</title>")},
		"assets/app-1.js":      {Data: []byte("console.log(1)")},
		"manifest.webmanifest": {Data: []byte(`{"name":"App"}`)},
	}
	ts := httptest.NewServer(server.New(cfg, conn, static, "test"))
	t.Cleanup(ts.Close)
	return ts, conn
}

// client is a browser-like API client with its own cookie jar.
type client struct {
	t       *testing.T
	base    string
	http    *http.Client
	headers map[string]string
}

func newClient(t *testing.T, ts *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: ts.URL, http: &http.Client{Jar: jar}, headers: map[string]string{}}
}

// do sends a JSON request and decodes a JSON response into out (if non-nil).
func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			c.t.Fatalf("%s %s: decode: %v", method, path, err)
		}
	}
	return resp.StatusCode
}

const password = "a long password"

func (c *client) signup(email string) {
	c.t.Helper()
	if code := c.do("POST", "/api/signup", map[string]string{"email": email, "password": password}, nil); code != 201 {
		c.t.Fatalf("signup %s: status %d", email, code)
	}
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

// send posts a raw body of the given content type, with extra headers.
func (c *client) send(path, contentType string, body []byte, headers map[string]string, out any) int {
	c.t.Helper()
	req, _ := http.NewRequest("POST", c.base+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			c.t.Fatalf("POST %s: decode: %v", path, err)
		}
	}
	return resp.StatusCode
}
