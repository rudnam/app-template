// Package config reads the server's settings from environment variables.
// README.md documents each one for whoever runs the app.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

type Config struct {
	// Addr is the listen address.
	Addr string
	// DatabaseURL is the PostgreSQL connection URL.
	DatabaseURL string
	LogLevel    slog.Level
	// SecureCookies sets the Secure flag on the session cookie. Turn it off
	// only for local development over plain http.
	SecureCookies bool
	// TrustProxy reads the client IP from the last X-Forwarded-For entry,
	// which the reverse proxy in front adds. Enable it only when the app is
	// reachable solely through that proxy.
	TrustProxy bool
	// PublicOrigin is the site's public address, e.g. https://app.example.com.
	// Requests from it pass the CSRF origin check even if a proxy rewrites Host.
	PublicOrigin string
	// AllowSignup lets new accounts be created.
	AllowSignup bool
	// AuthRateLimit is sign-in and sign-up attempts per minute per client IP.
	AuthRateLimit int
	// AndroidPackage and AndroidCertFingerprints, when set, publish
	// /.well-known/assetlinks.json so the Android app can open the site
	// full screen. See android/README.md.
	AndroidPackage          string
	AndroidCertFingerprints []string
}

// Load reads the configuration through getenv (os.Getenv in production) and
// reports every invalid setting at once.
func Load(getenv func(string) string) (Config, error) {
	r := reader{getenv: getenv}
	c := Config{
		Addr:                    r.str("ADDR", ":8080"),
		DatabaseURL:             r.required("DATABASE_URL"),
		LogLevel:                r.level("LOG_LEVEL", slog.LevelInfo),
		SecureCookies:           r.bool("COOKIE_SECURE", true),
		TrustProxy:              r.bool("TRUST_PROXY", false),
		PublicOrigin:            strings.TrimSuffix(r.str("PUBLIC_ORIGIN", ""), "/"),
		AllowSignup:             r.bool("ALLOW_SIGNUP", true),
		AuthRateLimit:           r.positiveInt("AUTH_RATE_LIMIT", 10),
		AndroidPackage:          r.str("ANDROID_PACKAGE", ""),
		AndroidCertFingerprints: r.list("ANDROID_CERT_FINGERPRINTS"),
	}
	if (c.AndroidPackage == "") != (len(c.AndroidCertFingerprints) == 0) {
		r.errs = append(r.errs, errors.New("set both ANDROID_PACKAGE and ANDROID_CERT_FINGERPRINTS, or neither"))
	}
	return c, errors.Join(r.errs...)
}

// reader collects parse errors so Load can report them all together.
type reader struct {
	getenv func(string) string
	errs   []error
}

func (r *reader) str(key, def string) string {
	if v := strings.TrimSpace(r.getenv(key)); v != "" {
		return v
	}
	return def
}

func (r *reader) required(key string) string {
	v := r.str(key, "")
	if v == "" {
		r.errs = append(r.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func (r *reader) bool(key string, def bool) bool {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s must be true or false, got %q", key, v))
	}
	return b
}

func (r *reader) positiveInt(key string, def int) int {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		r.errs = append(r.errs, fmt.Errorf("%s must be a positive whole number, got %q", key, v))
	}
	return n
}

func (r *reader) level(key string, def slog.Level) slog.Level {
	v := r.str(key, "")
	if v == "" {
		return def
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(v)); err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s must be debug, info, warn or error, got %q", key, v))
	}
	return l
}

// list reads a comma-separated value, dropping empty entries.
func (r *reader) list(key string) []string {
	var out []string
	for _, part := range strings.Split(r.getenv(key), ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
