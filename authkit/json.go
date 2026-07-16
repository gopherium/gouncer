// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gopherium/gouncer"
)

type errorResponse struct {
	Error string `json:"error"`
}

// Respond writes v as a JSON response with the given status code, falling back to a 500 error payload
// if marshaling fails.
func Respond(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// RespondError writes a JSON error response with the given status code and message.
func RespondError(w http.ResponseWriter, status int, message string) {
	Respond(w, status, errorResponse{Error: message})
}

// respondAuthError writes the mapped auth error, masking unrecognized errors as internal ones.
func respondAuthError(w http.ResponseWriter, err error) {
	if status, message, ok := StatusForAuthError(err); ok {
		RespondError(w, status, message)
		return
	}
	RespondError(w, http.StatusInternalServerError, "internal error")
}

// StatusForAuthError returns the HTTP status code and client-facing message
// for a gouncer error, reporting false for errors it does not recognize.
func StatusForAuthError(err error) (int, string, bool) {
	switch {
	case errors.Is(err, gouncer.ErrInvalidEmail):
		return http.StatusUnprocessableEntity, "invalid email address", true
	case errors.Is(err, gouncer.ErrEmptyName):
		return http.StatusUnprocessableEntity, "name is required", true
	case errors.Is(err, gouncer.ErrNameTooLong):
		return http.StatusUnprocessableEntity, "name must be at most 256 characters", true
	case errors.Is(err, gouncer.ErrWeakPassword):
		return http.StatusUnprocessableEntity, "password must be at least 12 characters", true
	case errors.Is(err, gouncer.ErrPasswordTooLong):
		return http.StatusUnprocessableEntity, "password must be at most 1024 characters", true
	case errors.Is(err, gouncer.ErrUserNotFound):
		return http.StatusNotFound, "user not found", true
	case errors.Is(err, gouncer.ErrEmailTaken):
		return http.StatusConflict, "email already in use", true
	default:
		return 0, "", false
	}
}

// MaxRequestBodyBytes caps how much of a request body Decode will read,
// so an unauthenticated caller cannot exhaust memory.
const MaxRequestBodyBytes = 1 << 20

// Decode reads and JSON-decodes a single request body into a value of
// type T, bounding the body size and rejecting trailing content.
func Decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&v); err != nil {
		return v, fmt.Errorf("decode json: %w", err)
	}
	if dec.More() {
		return v, errors.New("decode json: unexpected trailing content")
	}
	return v, nil
}
