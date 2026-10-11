// Package httpx is the HTTP plumbing every feature shares: handlers that
// return errors, JSON in and out, and middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
)

// Error is an error with a status code and a message that is safe to show
// the client. Return one from a HandlerFunc to answer with that status.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%d %s", e.Status, e.Message) }

// Errorf returns an *Error with a formatted message.
func Errorf(status int, format string, args ...any) *Error {
	return &Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// HandlerFunc is a handler that can fail. A returned *Error is sent as
// {"error": message} with its status; any other error is logged and answered
// with a plain 500, so internal details never reach the client.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

func (fn HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := fn(w, r)
	if err == nil {
		return
	}
	var e *Error
	if errors.As(err, &e) {
		WriteError(w, e.Status, e.Message)
		return
	}
	slog.ErrorContext(r.Context(), "internal error", "method", r.Method, "path", r.URL.Path, "err", err)
	WriteError(w, http.StatusInternalServerError, "internal error")
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// MaxBody is the default size limit for JSON request bodies.
const MaxBody = 64 << 10 // 64 KiB

// ReadJSON decodes a single JSON object of at most maxBytes into dst, refusing
// unknown fields. Requiring the JSON content type also means a plain HTML form
// on another site cannot send the request.
func ReadJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, dst any) error {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		return Errorf(http.StatusUnsupportedMediaType, "Content-Type must be application/json")
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			return Errorf(http.StatusRequestEntityTooLarge, "request body over %d bytes", maxBytes)
		case errors.Is(err, io.EOF):
			return Errorf(http.StatusBadRequest, "request body is empty")
		default:
			return Errorf(http.StatusBadRequest, "invalid JSON: %v", err)
		}
	}
	if dec.More() {
		return Errorf(http.StatusBadRequest, "request body must be a single JSON object")
	}
	return nil
}
