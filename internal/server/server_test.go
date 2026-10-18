package server_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/francis/app-template/internal/config"
)

func TestHealth(t *testing.T) {
	ts := newTestServer(t)
	resp, body := get(t, ts.URL+"/healthz")
	if resp.StatusCode != 200 || body != `{"status":"ok","version":"test"}`+"\n" {
		t.Fatalf("healthz: %d %s", resp.StatusCode, body)
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	ts := newTestServer(t)
	resp, body := get(t, ts.URL+"/api/nope")
	if resp.StatusCode != 404 || !strings.Contains(body, `"error"`) {
		t.Fatalf("got %d %s", resp.StatusCode, body)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestServesWebApp(t *testing.T) {
	ts := newTestServer(t)
	cases := []struct {
		path, wantBody, wantCache string
		wantStatus                int
	}{
		{"/", "<title>Landing</title>", "no-cache", 200},
		{"/app/", "<title>App</title>", "no-cache", 200},
		{"/app/notes/42", "<title>App</title>", "no-cache", 200}, // client-side route
		{"/app?x=1", "<title>App</title>", "no-cache", 200},      // redirected to /app/?x=1
		{"/assets", "404", "", 404},                              // no directory listings
		{"/notes/42", "404", "", 404},                            // not under /app/
		{"/assets/app-1.js", "console.log", "immutable", 200},
		{"/assets/missing.js", "404", "", 404},
	}
	for _, c := range cases {
		resp, body := get(t, ts.URL+c.path)
		if resp.StatusCode != c.wantStatus || !strings.Contains(body, c.wantBody) ||
			!strings.Contains(resp.Header.Get("Cache-Control"), c.wantCache) {
			t.Errorf("%s: %d %q cache %q", c.path, resp.StatusCode, body, resp.Header.Get("Cache-Control"))
		}
	}
	if resp, _ := get(t, ts.URL+"/"); !strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Error("no Content-Security-Policy")
	}
	if resp, _ := get(t, ts.URL+"/manifest.webmanifest"); resp.Header.Get("Content-Type") != "application/manifest+json" {
		t.Errorf("manifest Content-Type = %q", resp.Header.Get("Content-Type"))
	}
	req, _ := http.NewRequest("POST", ts.URL+"/", nil)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 405 {
		t.Errorf("POST /: %v %v, want 405", resp.StatusCode, err)
	}
}

func TestAndroidAssetLinks(t *testing.T) {
	if resp, _ := get(t, newTestServer(t).URL+"/.well-known/assetlinks.json"); resp.StatusCode != 404 {
		t.Errorf("without config: %d, want 404", resp.StatusCode)
	}
	ts := newTestServer(t, func(c *config.Config) {
		c.AndroidPackage = "com.example.app"
		c.AndroidCertFingerprints = []string{"AA:BB"}
	})
	resp, body := get(t, ts.URL+"/.well-known/assetlinks.json")
	var links []struct {
		Target struct {
			PackageName  string   `json:"package_name"`
			Fingerprints []string `json:"sha256_cert_fingerprints"`
		}
	}
	if err := json.Unmarshal([]byte(body), &links); err != nil || resp.StatusCode != 200 || len(links) != 1 ||
		links[0].Target.PackageName != "com.example.app" || links[0].Target.Fingerprints[0] != "AA:BB" {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
}
