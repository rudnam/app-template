// Package account handles sign-up, sign-in and sessions, and provides the
// RequireUser middleware that other features put in front of their routes.
//
// Sessions are random tokens in an HttpOnly cookie (or, for the Android
// app, a bearer token: see app.go); the database keeps only their hashes. Any app instance can serve any request, so the app scales
// out without sticky sessions.
package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/francis/app-template/internal/db"
	"github.com/francis/app-template/internal/httpx"
	"github.com/francis/app-template/internal/mailer"
)

const sessionCookie = "session"

type Config struct {
	// AppName is the app's name, as emails call it.
	AppName       string
	SecureCookies bool
	TrustProxy    bool
	AllowSignup   bool
	// AttemptsPerMinute limits sign-in and sign-up attempts per client IP.
	AttemptsPerMinute int
	SessionTTL        time.Duration
	// Google, if set, turns on Sign in with Google.
	Google *GoogleConfig
	// Mail, if set, sends a link to confirm each new account's email.
	// PublicOrigin is where those links point, e.g. https://app.example.com.
	Mail         mailer.Sender
	PublicOrigin string
	// AppScheme is the Android app's package, whose URL scheme Google
	// sign-in sends the app back to. Empty turns that off.
	AppScheme string
	// Demo, if set, lets visitors try a demo account it fills (demo.go),
	// which lasts DemoTTL (default a day).
	Demo    DemoFiller
	DemoTTL time.Duration
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
	if cfg.DemoTTL == 0 {
		cfg.DemoTTL = 24 * time.Hour
	}
	if cfg.AttemptsPerMinute <= 0 {
		cfg.AttemptsPerMinute = 10
	}
	if g := cfg.Google; g != nil {
		if g.AuthURL == "" {
			g.AuthURL = googleAuthURL
		}
		if g.TokenURL == "" {
			g.TokenURL = googleTokenURL
		}
	}
	return &Handler{cfg: cfg, store: st, limit: httpx.NewRateLimiter(cfg.AttemptsPerMinute, time.Minute), now: time.Now}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/signup", h.rateLimited(h.signup))
	mux.Handle("POST /api/login", h.rateLimited(h.login))
	mux.Handle("POST /api/logout", httpx.HandlerFunc(h.logout))
	mux.Handle("GET /api/me", h.RequireUser(httpx.HandlerFunc(h.me)))
	mux.Handle("DELETE /api/me", h.RequireUser(h.rateLimited(h.deleteMe)))
	mux.HandleFunc("GET /api/verify-email", h.verifyEmail)
	mux.Handle("POST /api/verify-email", h.RequireUser(h.rateLimited(h.resendConfirmation)))
	mux.Handle("GET /api/auth/options", httpx.HandlerFunc(h.options))
	mux.Handle("POST /api/auth/app-token", h.rateLimited(h.appToken))
	mux.Handle("POST /api/password/forgot", h.rateLimited(h.forgotPassword))
	mux.Handle("POST /api/password/reset", h.rateLimited(h.resetPassword))
	if h.cfg.Demo != nil {
		mux.Handle("POST /api/demo", h.rateLimited(h.startDemo))
	}
	if h.cfg.Google != nil {
		mux.HandleFunc("GET /api/auth/google", h.googleStart)
		mux.HandleFunc("GET /api/auth/google/callback", h.googleCallback)
	}
}

// ---------- Middleware for other features ----------

type ctxKey struct{}

// sessionToken returns the request's session token: the app's bearer
// token, else the browser's cookie, else "".
func sessionToken(r *http.Request) string {
	if t := bearerToken(r); t != "" {
		return t
	}
	if c, err := r.Cookie(sessionCookie); err == nil {
		return c.Value
	}
	return ""
}

// RequireUser answers 401 unless the request has a valid session, and makes
// the user available to next through UserFrom.
func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return httpx.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error {
		token := sessionToken(r)
		if token == "" {
			return httpx.Errorf(http.StatusUnauthorized, "sign in required")
		}
		u, err := h.store.userBySession(r.Context(), hashToken(token))
		if errors.Is(err, db.ErrNotFound) {
			if bearerToken(r) == "" {
				h.clearCookie(w)
			}
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
	// HasPassword is false for accounts that only sign in with Google.
	HasPassword bool `json:"has_password"`
	// ConfirmEmail asks the person to open the link we emailed them.
	ConfirmEmail bool `json:"confirm_email"`
	// Demo is a throwaway demo account, deleted after a day.
	Demo bool `json:"demo"`
	// Token is the new session's token, for the Android app only.
	Token string `json:"token,omitempty"`
}

func (h *Handler) meFor(u User) meResponse {
	return meResponse{Email: u.Email, HasPassword: u.HasPassword, ConfirmEmail: h.cfg.Mail != nil && !u.EmailVerified, Demo: u.Demo}
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
	if err != nil || addr.Address != e || len(e) > 254 || strings.HasSuffix(e, demoDomain) {
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
	token, err := h.startSession(w, r, id)
	if err != nil {
		return err
	}
	if h.cfg.Mail != nil {
		// Not fatal: the app offers to send the link again.
		if err := h.sendConfirmation(r.Context(), id, email, httpx.Language(r)); err != nil {
			slog.ErrorContext(r.Context(), "send confirmation email", "user", id, "err", err)
		}
	}
	me := h.meFor(User{ID: id, Email: email, HasPassword: true})
	me.Token = token
	httpx.WriteJSON(w, http.StatusCreated, me)
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
	if hash == "" { // signs in with Google only
		burnTime(in.Password)
		return wrong
	}
	ok, err := CheckPassword(hash, in.Password)
	if err != nil {
		return fmt.Errorf("user %d: %w", u.ID, err)
	}
	if !ok {
		return wrong
	}
	_ = h.store.deleteExpiredSessions(r.Context()) // housekeeping; failure is harmless
	token, err := h.startSession(w, r, u.ID)
	if err != nil {
		return err
	}
	me := h.meFor(u)
	me.Token = token
	httpx.WriteJSON(w, http.StatusOK, me)
	return nil
}

// startSession signs the user in: the browser gets a cookie, and the
// Android app gets the token back, to put in the response.
func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, userID int64) (appToken string, err error) {
	ttl := h.cfg.SessionTTL
	if fromApp(r) {
		ttl = appSessionTTL
	}
	return h.startSessionUntil(w, r, userID, h.now().Add(ttl))
}

// startSessionUntil is startSession for a session that ends at expires.
func (h *Handler) startSessionUntil(w http.ResponseWriter, r *http.Request, userID int64, expires time.Time) (appToken string, err error) {
	token, hash, err := newSessionToken()
	if err != nil {
		return "", err
	}
	if err := h.store.createSession(r.Context(), userID, hash, expires); err != nil {
		return "", err
	}
	if fromApp(r) {
		return token, nil
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(expires.Sub(h.now()).Seconds()),
		HttpOnly: true, // not readable from JavaScript
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	return "", nil
}

func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.cfg.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) error {
	if token := sessionToken(r); token != "" {
		// Ending a demo deletes it: nobody can come back to it.
		if u, err := h.store.userBySession(r.Context(), hashToken(token)); err == nil && u.Demo {
			if err := h.store.deleteUser(r.Context(), u.ID); err != nil {
				return err
			}
		}
		if err := h.store.deleteSession(r.Context(), hashToken(token)); err != nil {
			return err
		}
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	httpx.WriteJSON(w, http.StatusOK, h.meFor(UserFrom(r)))
	return nil
}

// deleteMe deletes the account and everything in it. It asks for the
// password again or, for an account without one, the email typed out, so a
// session left open somewhere can't do it by accident.
func (h *Handler) deleteMe(w http.ResponseWriter, r *http.Request) error {
	u := UserFrom(r)
	var in struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := httpx.ReadJSON(w, r, httpx.MaxBody, &in); err != nil {
		return err
	}
	if u.HasPassword {
		hash, err := h.store.passwordHash(r.Context(), u.ID)
		if err != nil {
			return err
		}
		if len(in.Password) > MaxPasswordLen {
			return httpx.Errorf(http.StatusForbidden, "the password is incorrect")
		}
		if ok, err := CheckPassword(hash, in.Password); err != nil {
			return fmt.Errorf("user %d: %w", u.ID, err)
		} else if !ok {
			return httpx.Errorf(http.StatusForbidden, "the password is incorrect")
		}
	} else if !strings.EqualFold(strings.TrimSpace(in.Email), u.Email) {
		return httpx.Errorf(http.StatusBadRequest, "type your email address to confirm")
	}
	if err := h.store.deleteUser(r.Context(), u.ID); err != nil {
		return err
	}
	slog.InfoContext(r.Context(), "account deleted", "user", u.ID)
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}
