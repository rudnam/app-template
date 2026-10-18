// Package account handles sign-up, sign-in and sessions, and provides the
// RequireUser middleware that other features put in front of their routes.
//
// Sessions are random tokens in an HttpOnly cookie; the database keeps only
// their hashes. Any app instance can serve any request, so the app scales
// out without sticky sessions.
package account

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/francis/app-template/internal/db"
	"github.com/francis/app-template/internal/httpx"
)

const sessionCookie = "session"

type Config struct {
	SecureCookies bool
	TrustProxy    bool
	AllowSignup   bool
	// AttemptsPerMinute limits sign-in and sign-up attempts per client IP.
	AttemptsPerMinute int
	SessionTTL        time.Duration
}

type Handler struct {
	cfg   Config
	store *Store
	limit *httpx.RateLimiter
	now   func() time.Time
}

func NewHandler(cfg Config, st *Store) *Handler {
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 30 * 24 * time.Hour
	}
	if cfg.AttemptsPerMinute <= 0 {
		cfg.AttemptsPerMinute = 10
	}
	return &Handler{cfg: cfg, store: st, limit: httpx.NewRateLimiter(cfg.AttemptsPerMinute, time.Minute), now: time.Now}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/signup", h.rateLimited(h.signup))
	mux.Handle("POST /api/login", h.rateLimited(h.login))
	mux.Handle("POST /api/logout", httpx.HandlerFunc(h.logout))
	mux.Handle("GET /api/me", h.RequireUser(httpx.HandlerFunc(h.me)))
}

// ---------- Middleware for other features ----------

type ctxKey struct{}

// RequireUser answers 401 unless the request has a valid session, and makes
// the user available to next through UserFrom.
func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			return httpx.Errorf(http.StatusUnauthorized, "sign in required")
		}
		u, err := h.store.userBySession(r.Context(), hashToken(c.Value))
		if errors.Is(err, db.ErrNotFound) {
			h.clearCookie(w)
			return httpx.Errorf(http.StatusUnauthorized, "session expired; sign in again")
		}
		if err != nil {
			return err
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
		return nil
	})
}

// UserFrom returns the signed-in user. Call it only behind RequireUser.
func UserFrom(r *http.Request) User {
	u, ok := r.Context().Value(ctxKey{}).(User)
	if !ok {
		panic("account.UserFrom called on a route without RequireUser")
	}
	return u
}

// ---------- Handlers ----------

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type meResponse struct {
	Email string `json:"email"`
}

func (h *Handler) rateLimited(fn httpx.HandlerFunc) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		if !h.limit.Allow(httpx.ClientIP(r, h.cfg.TrustProxy), h.now()) {
			return httpx.Errorf(http.StatusTooManyRequests, "too many attempts; wait a minute and try again")
		}
		return fn(w, r)
	}
}

func normalizeEmail(raw string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e || len(e) > 254 {
		return "", httpx.Errorf(http.StatusBadRequest, "enter a valid email address")
	}
	return e, nil
}

func (h *Handler) signup(w http.ResponseWriter, r *http.Request) error {
	if !h.cfg.AllowSignup {
		return httpx.Errorf(http.StatusForbidden, "sign-up is closed on this server")
	}
	var in credentials
	if err := httpx.ReadJSON(w, r, httpx.MaxBody, &in); err != nil {
		return err
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return err
	}
	if utf8.RuneCountInString(in.Password) < MinPasswordLen || len(in.Password) > MaxPasswordLen {
		return httpx.Errorf(http.StatusBadRequest, "password must be %d to %d characters", MinPasswordLen, MaxPasswordLen)
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return err
	}
	id, err := h.store.createUser(r.Context(), email, hash)
	if errors.Is(err, errEmailTaken) {
		return httpx.Errorf(http.StatusConflict, "an account with this email already exists")
	}
	if err != nil {
		return err
	}
	if err := h.startSession(w, r, id); err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, meResponse{Email: email})
	return nil
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) error {
	var in credentials
	if err := httpx.ReadJSON(w, r, httpx.MaxBody, &in); err != nil {
		return err
	}
	wrong := httpx.Errorf(http.StatusUnauthorized, "email or password is incorrect")
	if len(in.Password) > MaxPasswordLen {
		return wrong
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		burnTime(in.Password)
		return wrong
	}
	u, hash, err := h.store.userByEmail(r.Context(), email)
	if errors.Is(err, db.ErrNotFound) {
		burnTime(in.Password) // same timing as a wrong password
		return wrong
	}
	if err != nil {
		return err
	}
	ok, err := CheckPassword(hash, in.Password)
	if err != nil {
		return fmt.Errorf("user %d: %w", u.ID, err)
	}
	if !ok {
		return wrong
	}
	_ = h.store.deleteExpiredSessions(r.Context()) // housekeeping; failure is harmless
	if err := h.startSession(w, r, u.ID); err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, meResponse{Email: u.Email})
	return nil
}

func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token, hash, err := newSessionToken()
	if err != nil {
		return err
	}
	expires := h.now().Add(h.cfg.SessionTTL)
	if err := h.store.createSession(r.Context(), userID, hash, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(h.cfg.SessionTTL.Seconds()),
		HttpOnly: true, // not readable from JavaScript
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.cfg.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := h.store.deleteSession(r.Context(), hashToken(c.Value)); err != nil {
			return err
		}
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	httpx.WriteJSON(w, http.StatusOK, meResponse{Email: UserFrom(r).Email})
	return nil
}
