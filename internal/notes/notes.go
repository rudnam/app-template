// Package notes is the example feature: each user keeps a list of notes. It
// shows the shape of a feature end to end (migration, store, handler, tests,
// web page, browser test). Copy it for your first real feature, then delete
// it; README.md lists every file it touches.
package notes

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/francis/app-template/internal/account"
	"github.com/francis/app-template/internal/db"
	"github.com/francis/app-template/internal/httpx"
)

const maxBodyLen = 10_000

type Note struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ---------- Store: all of this feature's SQL ----------

// Store scopes every query by user ID, so one user can never read or change
// another user's notes.
type Store struct{ db *sql.DB }

func NewStore(conn *sql.DB) *Store { return &Store{db: conn} }

// List returns the user's notes, newest first.
func (s *Store) List(ctx context.Context, userID int64) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, body, created_at FROM notes WHERE user_id = $1 ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) Create(ctx context.Context, userID int64, body string) (Note, error) {
	n := Note{Body: body}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO notes (user_id, body) VALUES ($1, $2) RETURNING id, created_at`, userID, body).
		Scan(&n.ID, &n.CreatedAt)
	return n, err
}

// Delete removes a note. It returns db.ErrNotFound if the user has no such note.
func (s *Store) Delete(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notes WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return db.ErrNotFound
	}
	return nil
}

// ---------- HTTP ----------

type Handler struct{ store *Store }

func NewHandler(st *Store) *Handler { return &Handler{store: st} }

// Register adds the routes; requireUser guards the ones that need a session.
func (h *Handler) Register(mux *http.ServeMux, requireUser func(http.Handler) http.Handler) {
	mux.Handle("GET /api/notes", requireUser(httpx.HandlerFunc(h.list)))
	mux.Handle("POST /api/notes", requireUser(httpx.HandlerFunc(h.create)))
	mux.Handle("DELETE /api/notes/{id}", requireUser(httpx.HandlerFunc(h.delete)))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	notes, err := h.store.List(r.Context(), account.UserFrom(r).ID)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, notes)
	return nil
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Body string `json:"body"`
	}
	if err := httpx.ReadJSON(w, r, httpx.MaxBody, &in); err != nil {
		return err
	}
	body := strings.TrimSpace(in.Body)
	if body == "" || utf8.RuneCountInString(body) > maxBodyLen {
		return httpx.Errorf(http.StatusBadRequest, "a note must be 1 to %d characters", maxBodyLen)
	}
	n, err := h.store.Create(r.Context(), account.UserFrom(r).ID, body)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusCreated, n)
	return nil
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return httpx.Errorf(http.StatusNotFound, "no such note")
	}
	err = h.store.Delete(r.Context(), account.UserFrom(r).ID, id)
	if errors.Is(err, db.ErrNotFound) {
		return httpx.Errorf(http.StatusNotFound, "no such note")
	}
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
