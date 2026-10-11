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
	ID    int64
	Email string
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
		`SELECT id, email, password_hash FROM users WHERE email = $1`, email).
		Scan(&u.ID, &u.Email, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		err = db.ErrNotFound
	}
	return u, passwordHash, err
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
		SELECT u.id, u.email FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`, tokenHash).Scan(&u.ID, &u.Email)
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
