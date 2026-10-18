package account

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/francis/app-template/internal/db"
)

var errEmailTaken = errors.New("email already registered")

// User is the signed-in user, as handlers see it.
type User struct {
	ID            int64
	Email         string
	EmailVerified bool
	HasPassword   bool // false for people who only sign in with Google
	Demo          bool // a throwaway demo account (demo.go)
}

// Store holds the account SQL.
type Store struct{ db *sql.DB }

func NewStore(conn *sql.DB) *Store { return &Store{db: conn} }

func (s *Store) createUser(ctx context.Context, email, passwordHash string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id`,
		email, passwordHash).Scan(&id)
	if db.IsUniqueViolation(err) {
		return 0, errEmailTaken
	}
	return id, err
}

func (s *Store) userByEmail(ctx context.Context, email string) (u User, passwordHash string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, email_verified_at IS NOT NULL FROM users WHERE email = $1`, email).
		Scan(&u.ID, &u.Email, &passwordHash, &u.EmailVerified)
	u.HasPassword = passwordHash != ""
	if errors.Is(err, sql.ErrNoRows) {
		err = db.ErrNotFound
	}
	return u, passwordHash, err
}

// createGoogleUser adds an account that signs in with Google only.
func (s *Store) createGoogleUser(ctx context.Context, email, googleSub string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash, google_sub, email_verified_at) VALUES ($1, '', $2, now()) RETURNING id`,
		email, googleSub).Scan(&id)
	if db.IsUniqueViolation(err) {
		return 0, errEmailTaken
	}
	return id, err
}

func (s *Store) userByGoogle(ctx context.Context, googleSub string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email FROM users WHERE google_sub = $1`, googleSub).Scan(&u.ID, &u.Email)
	if errors.Is(err, sql.ErrNoRows) {
		err = db.ErrNotFound
	}
	return u, err
}

// googleSubByEmail returns the account with this email, the Google ID
// linked to it ("" if none), and whether its email is confirmed.
func (s *Store) googleSubByEmail(ctx context.Context, email string) (id int64, googleSub string, verified bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT id, coalesce(google_sub, ''), email_verified_at IS NOT NULL FROM users WHERE email = $1`, email).
		Scan(&id, &googleSub, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		err = db.ErrNotFound
	}
	return id, googleSub, verified, err
}

// linkGoogle links a Google ID to an account that has none yet, which
// confirms its email. If the email was not confirmed before, whoever set the
// password never proved they own the address, so the password and every
// session are dropped: only the Google account gets in from now on.
func (s *Store) linkGoogle(ctx context.Context, userID int64, googleSub string, verified bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE users SET google_sub = $2, email_verified_at = coalesce(email_verified_at, now()),
			password_hash = CASE WHEN $3 THEN password_hash ELSE '' END
		WHERE id = $1 AND google_sub IS NULL`, userID, googleSub, verified)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return errOtherGoogle // linked by a concurrent sign-in
	}
	if !verified {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// passwordHash returns the account's password hash ("" if it has none).
func (s *Store) passwordHash(ctx context.Context, userID int64) (string, error) {
	var hash string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash)
	return hash, err
}

// deleteUser deletes the account and, through ON DELETE CASCADE, everything
// it owns.
func (s *Store) deleteUser(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return err
}

// createEmailToken stores a confirmation link's token hash, replacing any
// earlier ones, so only the newest link works.
func (s *Store) createEmailToken(ctx context.Context, userID int64, email string, tokenHash []byte, expires time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM email_tokens WHERE user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO email_tokens (token_hash, user_id, email, expires_at) VALUES ($1, $2, $3, $4)`,
		tokenHash, userID, email, expires); err != nil {
		return err
	}
	return tx.Commit()
}

// verifyEmail uses up a confirmation token and marks its address confirmed,
// if the token is unexpired and the account still has that address.
func (s *Store) verifyEmail(ctx context.Context, tokenHash []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID int64
	err = tx.QueryRowContext(ctx, `
		DELETE FROM email_tokens t USING users u
		WHERE t.token_hash = $1 AND t.user_id = u.id AND t.email = u.email AND t.expires_at > now()
		RETURNING t.user_id`, tokenHash).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return db.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET email_verified_at = coalesce(email_verified_at, now()) WHERE id = $1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) createSession(ctx context.Context, userID int64, tokenHash []byte, expires time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		tokenHash, userID, expires)
	return err
}

// userBySession returns the user that owns an unexpired session.
func (s *Store) userBySession(ctx context.Context, tokenHash []byte) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.email, u.email_verified_at IS NOT NULL, u.password_hash <> '', u.demo_expires_at IS NOT NULL
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash).Scan(&u.ID, &u.Email, &u.EmailVerified, &u.HasPassword, &u.Demo)
	if errors.Is(err, sql.ErrNoRows) {
		return u, db.ErrNotFound
	}
	return u, err
}

func (s *Store) deleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
}

func (s *Store) deleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= now()`)
	return err
}
