package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerFuncErrors(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
		wantBody   string
	}{
		{nil, 200, ""},
		{Errorf(404, "no such %s", "thing"), 404, `{"error":"no such thing"}`},
		{errors.New("db exploded"), 500, `{"error":"internal error"}`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		HandlerFunc(func(http.ResponseWriter, *http.Request) error { return c.err }).
			ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if rec.Code != c.wantStatus || strings.TrimSpace(rec.Body.String()) != c.wantBody {
			t.Errorf("%v: got %d %q", c.err, rec.Code, rec.Body)
		}
	}
}

func TestReadJSON(t *testing.T) {
	cases := []struct {
		contentType, body string
		wantStatus        int // 0 means success
	}{
		{"application/json", `{"name":"a"}`, 0},
		{"application/json; charset=utf-8", `{"name":"a"}`, 0},
		{"text/plain", `{"name":"a"}`, 415},
		{"application/json", ``, 400},
		{"application/json", `{"name":"a","extra":1}`, 400},
		{"application/json", `{"name":"a"} {}`, 400},
		{"application/json", `{"name":"` + strings.Repeat("x", 100) + `"}`, 413},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/", strings.NewReader(c.body))
		r.Header.Set("Content-Type", c.contentType)
		var dst struct{ Name string }
		err := ReadJSON(httptest.NewRecorder(), r, 64, &dst)
		var e *Error
		switch {
		case c.wantStatus == 0 && err != nil:
			t.Errorf("%q: unexpected error %v", c.body, err)
		case c.wantStatus != 0 && (!errors.As(err, &e) || e.Status != c.wantStatus):
			t.Errorf("%q %q: got %v, want status %d", c.contentType, c.body, err, c.wantStatus)
		}
	}
}

func TestCheckOrigin(t *testing.T) {
	h := CheckOrigin("https://app.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cases := []struct {
		method, origin, fetchSite string
		want                      int
	}{
		{"GET", "https://evil.example", "", 200},
		{"POST", "", "", 200}, // non-browser clients
		{"POST", "http://example.com", "", 200},
		{"POST", "https://app.example.com", "", 200},
		{"POST", "https://evil.example", "", 403},
		{"DELETE", "", "cross-site", 403},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "http://example.com/api/x", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.fetchSite != "" {
			r.Header.Set("Sec-Fetch-Site", c.fetchSite)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != c.want {
			t.Errorf("%s from %q/%q: %d, want %d", c.method, c.origin, c.fetchSite, rec.Code, c.want)
		}
	}
}

func TestRecover(t *testing.T) {
	rec := httptest.NewRecorder()
	Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })).
		ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 500 {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "6.6.6.6, 1.2.3.4")
	if got := ClientIP(r, false); got != "10.0.0.1" {
		t.Errorf("untrusted: %s", got)
	}
	if got := ClientIP(r, true); got != "1.2.3.4" {
		t.Errorf("X-Forwarded-For: %s", got)
	}
	// Only Cloudflare could be trusted to set this; a visitor can send it too.
	r.Header.Set("CF-Connecting-IP", "5.5.5.5")
	if got := ClientIP(r, true); got != "1.2.3.4" {
		t.Errorf("with CF-Connecting-IP sent: %s, want it ignored", got)
	}
}

func TestRateLimiter(t *testing.T) {
	l := NewRateLimiter(2, time.Minute)
	now := time.Now()
	got := []bool{l.Allow("a", now), l.Allow("a", now), l.Allow("a", now)}
	if !got[0] || !got[1] || got[2] {
		t.Fatalf("first three attempts = %v, want a burst of 2", got)
	}
	if !l.Allow("b", now) {
		t.Fatal("keys are not independent")
	}
	if !l.Allow("a", now.Add(30*time.Second)) {
		t.Fatal("no refill after half the window")
	}
}

func TestLanguage(t *testing.T) {
	for header, want := range map[string]string{
		"":                           "en",
		"ja":                         "ja",
		"ja-JP,ja;q=0.9,en-US;q=0.8": "ja",
		"en-US,en;q=0.9,ja;q=0.8":    "en",
		"fr-FR, ja;q=0.5":            "en",
		"en;q=0.4, ja-JP;q=0.7":      "ja",
		"*":                          "en",
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept-Language", header)
		if got := Language(r); got != want {
			t.Errorf("Language(%q) = %q, want %q", header, got, want)
		}
	}
}
