package account

// Resetting a forgotten password: the sign-in page asks for the email, we
// email a link to /app/#/reset?token=…, and the page there sets a new
// password. The token is after the #, so browsers never send it to the
// server or its proxies' logs. Setting the password signs out every other
// session and confirms the email, since the link proves the person gets
// mail there. Only offered when the server sends email.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/francis/app-template/internal/db"
	"github.com/francis/app-template/internal/httpx"
	"github.com/francis/app-template/internal/mailer"
)

const (
	resetTTL = time.Hour
	// resetGap is how soon another link may be sent to the same account,
	// so nobody can flood someone's inbox through the form.
	resetGap = time.Minute
)

// forgotPassword emails a reset link if an account uses the address. It
// answers the same either way, and sends in the background so the time it
// takes doesn't tell either: nobody can find out who has an account here.
func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) error {
	if h.cfg.Mail == nil {
		return httpx.Errorf(http.StatusNotFound, "this server doesn't send email")
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := httpx.ReadJSON(w, r, httpx.MaxBody, &in); err != nil {
		return err
	}
	email, err := normalizeEmail(in.Email)
	if err != nil {
		return err
	}
	ctx, lang := context.WithoutCancel(r.Context()), httpx.Language(r)
	go func() {
		if err := h.sendReset(ctx, email, lang); err != nil {
			slog.ErrorContext(ctx, "send password reset email", "err", err)
		}
	}()
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// sendReset emails a reset link to email's account, if there is one and no
// link went to it in the last resetGap.
func (h *Handler) sendReset(ctx context.Context, email, lang string) error {
	u, _, err := h.store.userByEmail(ctx, email)
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	token, hash, err := newSessionToken() // same shape: random token, stored hashed
	if err != nil {
		return err
	}
	if sent, err := h.store.createPasswordToken(ctx, u.ID, email, hash, h.now().Add(resetTTL)); err != nil || !sent {
		return err
	}
	link := h.cfg.PublicOrigin + "/app/#/reset?token=" + token
	if lang == "ja" {
		return h.cfg.Mail.Send(ctx, mailer.Message{
			To:      email,
			Subject: h.cfg.AppName + "のパスワードの再設定",
			Body: fmt.Sprintf("こんにちは。\n\n次のリンクを開いて、新しいパスワードを設定してください。\n\n%s\n\n"+
				"リンクの有効期限は1時間で、一度だけ使えます。パスワードの再設定を依頼した覚えがない場合は、このメールは無視してください。パスワードは変わりません。\n",
				link),
		})
	}
	return h.cfg.Mail.Send(ctx, mailer.Message{
		To:      email,
		Subject: "Reset your " + h.cfg.AppName + " password",
		Body: fmt.Sprintf("Hello,\n\nTo choose a new password, open this link:\n\n%s\n\n"+
			"The link works once, for an hour. If you didn't ask to reset your password, you can ignore this email; your password stays the same.\n",
			link),
	})
}

// resetPassword sets a new password with the token from the link, and signs
// in with it.
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) error {
	if h.cfg.Mail == nil {
		return httpx.Errorf(http.StatusNotFound, "this server doesn't send email")
	}
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := httpx.ReadJSON(w, r, httpx.MaxBody, &in); err != nil {
		return err
	}
	if utf8.RuneCountInString(in.Password) < MinPasswordLen || len(in.Password) > MaxPasswordLen {
		return httpx.Errorf(http.StatusBadRequest, "password must be %d to %d characters", MinPasswordLen, MaxPasswordLen)
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return err
	}
	u, err := h.store.resetPassword(r.Context(), hashToken(in.Token), hash)
	if errors.Is(err, db.ErrNotFound) {
		return httpx.Errorf(http.StatusBadRequest, "this link has expired or was already used; ask for a new one")
	}
	if err != nil {
		return err
	}
	slog.InfoContext(r.Context(), "password reset", "user", u.ID)
	token, err := h.startSession(w, r, u.ID)
	if err != nil {
		return err
	}
	me := h.meFor(u)
	me.Token = token
	httpx.WriteJSON(w, http.StatusOK, me)
	return nil
}

// createPasswordToken stores a reset link's token hash, replacing any
// earlier ones so only the newest link works. It stores nothing, and
// returns false, if a link was made for the account in the last resetGap.
func (s *Store) createPasswordToken(ctx context.Context, userID int64, email string, tokenHash []byte, expires time.Time) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// Locking the user's row makes two requests at once take turns.
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, userID); err != nil {
		return false, err
	}
	var recent bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM password_tokens WHERE user_id = $1 AND created_at > now() - make_interval(secs => $2))`,
		userID, resetGap.Seconds()).Scan(&recent); err != nil || recent {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_tokens WHERE user_id = $1`, userID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO password_tokens (token_hash, user_id, email, expires_at) VALUES ($1, $2, $3, $4)`,
		tokenHash, userID, email, expires); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// resetPassword uses up a reset token, if it is unexpired and the account
// still has the address it went to: it sets the password, confirms the
// email, and ends every session.
func (s *Store) resetPassword(ctx context.Context, tokenHash []byte, passwordHash string) (User, error) {
	u := User{EmailVerified: true, HasPassword: true}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return u, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `
		DELETE FROM password_tokens t USING users u
		WHERE t.token_hash = $1 AND t.user_id = u.id AND t.email = u.email AND t.expires_at > now()
		RETURNING u.id, u.email`, tokenHash).Scan(&u.ID, &u.Email)
	if errors.Is(err, sql.ErrNoRows) {
		return u, db.ErrNotFound
	}
	if err != nil {
		return u, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET password_hash = $2, email_verified_at = coalesce(email_verified_at, now()) WHERE id = $1`,
		u.ID, passwordHash); err != nil {
		return u, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, u.ID); err != nil {
		return u, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_tokens WHERE user_id = $1`, u.ID); err != nil {
		return u, err
	}
	return u, tx.Commit()
}
