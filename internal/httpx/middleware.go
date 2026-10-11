package httpx

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Chain wraps h in middleware; the first one listed is the outermost.
func Chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// SecurityHeaders sets a strict Content-Security-Policy and related headers,
// and keeps API responses out of caches.
func SecurityHeaders(next http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self'",
		"style-src 'self'",
		"img-src 'self' data:",
		"connect-src 'self'",
		"manifest-src 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"base-uri 'none'",
		"form-action 'self'",
	}, "; ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// CheckOrigin refuses cross-site requests that change data (CSRF). The
// session cookie is SameSite=Lax already; this is a second, independent check.
// publicOrigin (may be empty) is accepted even if a proxy rewrites Host.
func CheckOrigin(publicOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if origin := r.Header.Get("Origin"); origin != "" {
					if !sameOrigin(origin, r.Host, publicOrigin) {
						WriteError(w, http.StatusForbidden, "cross-origin request refused")
						return
					}
				} else if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
					WriteError(w, http.StatusForbidden, "cross-site request refused")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func sameOrigin(origin, host, publicOrigin string) bool {
	if u, err := url.Parse(origin); err == nil && u.Host == host {
		return true
	}
	return publicOrigin != "" && strings.EqualFold(origin, publicOrigin)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// LogRequests logs one line per request, except health checks.
func LogRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/healthz" {
			return
		}
		slog.InfoContext(r.Context(), "request", "method", r.Method, "path", r.URL.Path,
			"status", rec.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

// Recover turns a panic in a handler into a logged 500.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.ErrorContext(r.Context(), "panic", "method", r.Method, "path", r.URL.Path, "value", v)
				WriteError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
