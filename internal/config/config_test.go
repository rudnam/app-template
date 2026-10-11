package config

import (
	"log/slog"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || !c.SecureCookies || c.TrustProxy || !c.AllowSignup ||
		c.AuthRateLimit != 10 || c.LogLevel != slog.LevelInfo {
		t.Errorf("defaults = %+v", c)
	}
}

func TestParsesValues(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL":              "postgres://x",
		"COOKIE_SECURE":             "false",
		"PUBLIC_ORIGIN":             "https://app.example.com/",
		"LOG_LEVEL":                 "debug",
		"AUTH_RATE_LIMIT":           "1000",
		"ANDROID_PACKAGE":           "com.example.app",
		"ANDROID_CERT_FINGERPRINTS": " AA:BB , CC:DD,",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.SecureCookies || c.PublicOrigin != "https://app.example.com" || c.LogLevel != slog.LevelDebug ||
		c.AuthRateLimit != 1000 || len(c.AndroidCertFingerprints) != 2 || c.AndroidCertFingerprints[1] != "CC:DD" {
		t.Errorf("parsed = %+v", c)
	}
}

func TestReportsEveryError(t *testing.T) {
	_, err := Load(env(map[string]string{
		"COOKIE_SECURE":   "maybe",
		"AUTH_RATE_LIMIT": "0",
		"LOG_LEVEL":       "loud",
		"ANDROID_PACKAGE": "com.example.app",
	}))
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"DATABASE_URL", "COOKIE_SECURE", "AUTH_RATE_LIMIT", "LOG_LEVEL", "ANDROID_CERT_FINGERPRINTS"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}
