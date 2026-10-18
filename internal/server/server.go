// Package server assembles the HTTP handler: it wires each feature's routes
// and wraps them in the shared middleware. A new feature gets one line here.
package server

import (
	"context"
	"database/sql"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/francis/app-template/internal/account"
	"github.com/francis/app-template/internal/config"
	"github.com/francis/app-template/internal/demo"
	"github.com/francis/app-template/internal/httpx"
	"github.com/francis/app-template/internal/mailer"
	"github.com/francis/app-template/internal/notes"
)

// appName is what emails call the app.
const appName = "App Template"

func init() {
	// The runtime image has no /etc/mime.types, and Go doesn't know this one.
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// New returns the app's complete HTTP handler. static is the built web app;
// version is reported by /healthz so you can tell which build is running.
func New(cfg config.Config, conn *sql.DB, static fs.FS, version string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", health(conn, version))
	if cfg.AndroidPackage != "" {
		mux.Handle("GET /.well-known/assetlinks.json", assetLinks(cfg.AndroidPackage, cfg.AndroidCertFingerprints))
	}

	// Features
	var google *account.GoogleConfig
	if cfg.GoogleClientID != "" {
		google = &account.GoogleConfig{
			ClientID:     cfg.GoogleClientID,
			ClientSecret: cfg.GoogleClientSecret,
			RedirectURL:  cfg.PublicOrigin + "/api/auth/google/callback",
			AuthURL:      cfg.GoogleAuthURL,
			TokenURL:     cfg.GoogleTokenURL,
		}
	}
	sender := cfg.Mailer
	if sender == nil && cfg.SMTPHost != "" {
		sender = &mailer.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword, From: cfg.MailFrom}
	}
	notesStore := notes.NewStore(conn)
	var demoFiller account.DemoFiller
	if cfg.Demo {
		demoFiller = demo.Filler(notesStore)
	}
	accounts := account.NewHandler(account.Config{
		AppName:           appName,
		SecureCookies:     cfg.SecureCookies,
		TrustProxy:        cfg.TrustProxy,
		AllowSignup:       cfg.AllowSignup,
		AttemptsPerMinute: cfg.AuthRateLimit,
		Google:            google,
		Mail:              sender,
		PublicOrigin:      cfg.PublicOrigin,
		AppScheme:         cfg.AndroidPackage,
		Demo:              demoFiller,
	}, account.NewStore(conn))
	accounts.Register(mux)
	notes.NewHandler(notesStore).Register(mux, accounts.RequireUser)

	// Unknown API paths get a JSON 404 rather than the web page.
	mux.Handle("/api/", httpx.HandlerFunc(func(http.ResponseWriter, *http.Request) error {
		return httpx.Errorf(http.StatusNotFound, "not found")
	}))
	mux.Handle("/", webApp(static))

	return httpx.Chain(mux,
		httpx.LogRequests,
		httpx.Recover,
		httpx.SecurityHeaders,
		httpx.AllowApp,
		httpx.CheckOrigin(cfg.PublicOrigin),
	)
}

// health answers 200 when the database is reachable. Docker, load balancers
// and uptime checks all use it.
func health(conn *sql.DB, version string) http.Handler {
	return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := conn.PingContext(ctx); err != nil {
			return httpx.Errorf(http.StatusServiceUnavailable, "database unreachable")
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
		return nil
	})
}

// webApp serves the built web app. The introduction page (landing.html) is
// at /, and the single-page app (index.html) at /app/: paths under /app/ that
// are not files get index.html, so client-side routes survive a reload. Vite
// puts content-hashed files in assets/, which can be cached for good;
// everything else is revalidated, so a deploy shows up on the next load.
func webApp(static fs.FS) http.Handler {
	files := http.FileServerFS(static)
	index, err := fs.ReadFile(static, "index.html")
	if err != nil {
		index = []byte("<!doctype html><title>Missing build</title>")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			httpx.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		switch {
		case name == "" || name == "landing.html":
			http.ServeFileFS(w, r, static, "landing.html")
		case r.URL.Path == "/app":
			to := "/app/"
			if r.URL.RawQuery != "" {
				to += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, to, http.StatusMovedPermanently)
		case name == "app" || strings.HasPrefix(name, "app/") || name == "index.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(index)
		case path.Ext(name) == "":
			// Not a file, and no page of ours: no directory listings.
			http.NotFound(w, r)
		default:
			files.ServeHTTP(w, r)
		}
	})
}

// assetLinks proves to Android that the app with this package and signing
// key belongs to this site, so it can show the site without browser UI.
func assetLinks(pkg string, fingerprints []string) http.Handler {
	body := []map[string]any{{
		"relation": []string{"delegate_permission/common.handle_all_urls"},
		"target": map[string]any{
			"namespace":                "android_app",
			"package_name":             pkg,
			"sha256_cert_fingerprints": fingerprints,
		},
	}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, body)
	})
}
