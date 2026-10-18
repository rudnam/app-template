package account

// Sign in with Google (OpenID Connect, authorization code flow with PKCE).
// It asks only for "openid email", which needs no Google verification.
//
// The ID token comes straight from Google's token endpoint over TLS in
// exchange for our client secret, so, as OpenID Connect allows, its claims
// are trusted without checking its signature (no key fetching, no library).

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/francis/app-template/internal/db"
	"github.com/francis/app-template/internal/httpx"
)

// GoogleConfig turns on Sign in with Google. RedirectURL must be registered
// with the OAuth client, e.g. https://app.example.com/api/auth/google/callback.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// AuthURL and TokenURL default to Google's; tests point them at a fake.
	AuthURL  string
	TokenURL string
}

const (
	googleCookie   = "google_auth"
	googleIssuer   = "https://accounts.google.com"
	googleAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"
)

var googleHTTP = &http.Client{Timeout: 10 * time.Second}

type optionsResponse struct {
	Google bool `json:"google"`
	// Demo: visitors can try a demo without an account (POST /api/demo).
	Demo bool `json:"demo"`
	// Reset: a forgotten password can be reset by email (reset.go).
	Reset bool `json:"reset"`
}

// options tells the sign-in page which ways to sign in exist.
func (h *Handler) options(w http.ResponseWriter, r *http.Request) error {
	httpx.WriteJSON(w, http.StatusOK, optionsResponse{Google: h.cfg.Google != nil, Demo: h.cfg.Demo != nil, Reset: h.cfg.Mail != nil})
	return nil
}

// googleStart sends the browser to Google, remembering in a short-lived
// cookie what the callback needs to check that the answer is for us. With
// app_challenge, the Android app started it (see app.go).
func (h *Handler) googleStart(w http.ResponseWriter, r *http.Request) {
	g := h.cfg.Google
	state, nonce, verifier := randomString(), randomString(), randomString()
	app := r.URL.Query().Get("app_challenge")
	if app != "" && (h.cfg.AppScheme == "" || appChallenge(app) == nil) {
		httpx.WriteError(w, http.StatusBadRequest, "this server can't sign in the app with Google")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     googleCookie,
		Value:    state + "." + nonce + "." + verifier + "." + app,
		Path:     "/api/auth/google",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode, // sent on the redirect back from Google
	})
	challenge := sha256.Sum256([]byte(verifier))
	q := url.Values{
		"client_id":             {g.ClientID},
		"redirect_uri":          {g.RedirectURL},
		"response_type":         {"code"},
		"scope":                 {"openid email"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	http.Redirect(w, r, g.AuthURL+"?"+q.Encode(), http.StatusFound)
}

// googleCallback finishes the sign-in and sends the browser back to the web
// app, or to the Android app with a code, with signin_error=<reason> if it
// failed. The page knows the wording.
func (h *Handler) googleCallback(w http.ResponseWriter, r *http.Request) {
	reason, appCode, app, err := h.googleSignIn(w, r)
	if err != nil {
		slog.ErrorContext(r.Context(), "google sign-in", "err", err)
	}
	if app {
		q := url.Values{"code": {appCode}}
		if reason != "" {
			q = url.Values{"signin_error": {reason}}
		}
		http.Redirect(w, r, h.appRedirect(q), http.StatusFound)
		return
	}
	target := "/app/"
	if reason != "" {
		target = "/app/?signin_error=" + reason
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// googleSignIn returns a reason code for the page when sign-in fails, and
// an error too when the failure is ours and worth logging. When the Android
// app started it (app is true), it returns a one-time code for the app
// instead of starting a session here.
func (h *Handler) googleSignIn(w http.ResponseWriter, r *http.Request) (reason, appCode string, app bool, err error) {
	q := r.URL.Query()
	c, cookieErr := r.Cookie(googleCookie)
	http.SetCookie(w, &http.Cookie{
		Name: googleCookie, Value: "", Path: "/api/auth/google", MaxAge: -1,
		HttpOnly: true, Secure: h.cfg.SecureCookies, SameSite: http.SameSiteLaxMode,
	})
	var parts []string
	if cookieErr == nil {
		parts = strings.Split(c.Value, ".")
	}
	var challenge []byte
	if len(parts) == 4 && parts[3] != "" {
		challenge, app = appChallenge(parts[3]), true
	}
	if !h.limit.Allow(httpx.ClientIP(r, h.cfg.TrustProxy), h.now()) {
		return "busy", "", app, nil
	}
	if q.Get("error") == "access_denied" {
		return "cancelled", "", app, nil
	}
	if len(parts) != 4 || subtle.ConstantTimeCompare([]byte(parts[0]), []byte(q.Get("state"))) != 1 || q.Get("code") == "" ||
		(app && challenge == nil) {
		return "expired", "", app, nil // stale tab, a second click, or someone else's link
	}
	nonce, verifier := parts[1], parts[2]

	claims, err := h.googleExchange(r.Context(), q.Get("code"), verifier)
	if err != nil {
		return "failed", "", app, err
	}
	if claims.Nonce != nonce {
		return "failed", "", app, errors.New("id token nonce does not match")
	}
	if !claims.EmailVerified {
		return "unverified", "", app, nil
	}
	email, err := normalizeEmail(claims.Email)
	if err != nil {
		return "failed", "", app, fmt.Errorf("unusable email from google: %q", claims.Email)
	}

	id, err := h.googleUser(r.Context(), claims.Sub, email)
	if errors.Is(err, errSignupClosed) {
		return "closed", "", app, nil
	}
	if errors.Is(err, errOtherGoogle) {
		return "linked", "", app, nil
	}
	if err != nil {
		return "failed", "", app, err
	}
	if app {
		appCode, err = h.store.createAppCode(r.Context(), id, challenge, h.now().Add(appCodeTTL))
		if err != nil {
			return "failed", "", app, err
		}
		return "", appCode, app, nil
	}
	if _, err := h.startSession(w, r, id); err != nil {
		return "failed", "", app, err
	}
	return "", "", app, nil
}

var (
	errSignupClosed = errors.New("sign-up is closed")
	errOtherGoogle  = errors.New("account is linked to another google account")
)

// googleUser finds the account for a Google identity: the one already
// linked to it, else the one with its (Google-verified) email, which gets
// linked, else a new one.
func (h *Handler) googleUser(ctx context.Context, sub, email string) (int64, error) {
	u, err := h.store.userByGoogle(ctx, sub)
	if err == nil {
		return u.ID, nil
	}
	if !errors.Is(err, db.ErrNotFound) {
		return 0, err
	}
	id, linked, verified, err := h.store.googleSubByEmail(ctx, email)
	switch {
	case err == nil && linked == "":
		return id, h.store.linkGoogle(ctx, id, sub, verified)
	case err == nil:
		return 0, errOtherGoogle
	case !errors.Is(err, db.ErrNotFound):
		return 0, err
	}
	if !h.cfg.AllowSignup {
		return 0, errSignupClosed
	}
	return h.store.createGoogleUser(ctx, email, sub)
}

type googleClaims struct {
	Iss           string `json:"iss"`
	Aud           string `json:"aud"`
	Exp           int64  `json:"exp"`
	Sub           string `json:"sub"`
	Nonce         string `json:"nonce"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

// googleExchange trades the code for an ID token and returns its checked claims.
func (h *Handler) googleExchange(ctx context.Context, code, verifier string) (googleClaims, error) {
	g := h.cfg.Google
	var claims googleClaims
	form := url.Values{
		"code":          {code},
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"redirect_uri":  {g.RedirectURL},
		"grant_type":    {"authorization_code"},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return claims, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := googleHTTP.Do(req)
	if err != nil {
		return claims, err
	}
	defer resp.Body.Close()
	var body struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 1<<20)).Decode(&body); err != nil {
		return claims, fmt.Errorf("token response (%d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || body.IDToken == "" {
		return claims, fmt.Errorf("token endpoint: %d %s", resp.StatusCode, body.Error)
	}

	parts := strings.Split(body.IDToken, ".")
	if len(parts) != 3 {
		return claims, errors.New("id token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, fmt.Errorf("id token payload: %w", err)
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, fmt.Errorf("id token claims: %w", err)
	}
	switch {
	case claims.Iss != googleIssuer && claims.Iss != "accounts.google.com":
		return claims, fmt.Errorf("id token issuer %q", claims.Iss)
	case claims.Aud != g.ClientID:
		return claims, fmt.Errorf("id token audience %q", claims.Aud)
	case time.Unix(claims.Exp, 0).Before(h.now()):
		return claims, errors.New("id token expired")
	case claims.Sub == "":
		return claims, errors.New("id token has no subject")
	}
	return claims, nil
}

func randomString() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // never fails on supported platforms
	return base64.RawURLEncoding.EncodeToString(b)
}
