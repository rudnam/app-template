package account

// Demo accounts: "Try the demo" signs a visitor in to a throwaway account
// of their own, filled with sample data (Config.Demo, see internal/demo),
// so they can see the app in use without making an account. It has no
// password, so nobody can sign in to it again; its session ends after
// Config.DemoTTL, and expired demos are deleted, with everything in them,
// whenever a new one starts or the visitor ends theirs. Demos can't reach
// outside the app: features that would (access tokens) refuse them through
// NotInDemo.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/francis/app-template/internal/httpx"
)

// DemoFiller fills a new demo account, named in lang ("en" or "ja") and
// dated in the visitor's time zone.
type DemoFiller func(ctx context.Context, userID int64, lang string, zone *time.Location) error

// demoDomain is reserved (RFC 2606), so a demo's made-up address can never
// be anyone's real one.
const demoDomain = "@demo.invalid"

// NotInDemo refuses what a demo account can't do. Call it behind RequireUser.
func NotInDemo(r *http.Request) error {
	if UserFrom(r).Demo {
		return httpx.Errorf(http.StatusForbidden, "the demo can't do this; create an account to use it")
	}
	return nil
}

func (h *Handler) startDemo(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		// TZ is the visitor's time zone (an IANA name), for the sample records' times.
		TZ string `json:"tz"`
	}
	if err := httpx.ReadJSON(w, r, httpx.MaxBody, &in); err != nil {
		return err
	}
	zone, err := time.LoadLocation(in.TZ)
	if err != nil || in.TZ == "" {
		zone = time.UTC
	}
	if err := h.store.deleteExpiredDemos(r.Context()); err != nil {
		slog.WarnContext(r.Context(), "delete expired demos", "err", err) // housekeeping; carry on
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	expires := h.now().Add(h.cfg.DemoTTL)
	id, err := h.store.createDemoUser(r.Context(), "demo-"+hex.EncodeToString(random)+demoDomain, expires)
	if err != nil {
		return err
	}
	if err := h.cfg.Demo(r.Context(), id, httpx.Language(r), zone); err != nil {
		_ = h.store.deleteUser(context.WithoutCancel(r.Context()), id)
		return fmt.Errorf("fill demo %d: %w", id, err)
	}
	token, err := h.startSessionUntil(w, r, id, expires)
	if err != nil {
		return err
	}
	me := h.meFor(User{ID: id, EmailVerified: true, Demo: true})
	me.Token = token
	httpx.WriteJSON(w, http.StatusCreated, me)
	return nil
}

func (s *Store) createDemoUser(ctx context.Context, email string, expires time.Time) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, email_verified_at, demo_expires_at) VALUES ($1, '', now(), $2) RETURNING id`,
		email, expires).Scan(&id)
	return id, err
}

// deleteExpiredDemos deletes demo accounts whose time is up, and through ON
// DELETE CASCADE everything in them.
func (s *Store) deleteExpiredDemos(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE demo_expires_at <= now()`)
	return err
}
