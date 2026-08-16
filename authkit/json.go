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
	Error string         `json:"error"`
	Code  string         `json:"code,omitempty"`
	Meta  map[string]any `json:"meta,omitempty"`
}

// Refusal is the reason a request was turned away, named for a client that translates it.
type Refusal struct {
	Message string
	Code    string
	Meta    map[string]any
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

// RespondRefusal writes a JSON error response naming the reason as a code beside its message.
func RespondRefusal(w http.ResponseWriter, status int, refusal Refusal) {
	Respond(w, status, errorResponse{Error: refusal.Message, Code: refusal.Code, Meta: refusal.Meta})
}

// respondAuthError writes the mapped auth error, masking unrecognized errors as internal ones.
func respondAuthError(w http.ResponseWriter, err error) {
	if status, refusal, ok := RefusalForAuthError(err); ok {
		RespondRefusal(w, status, refusal)
		return
	}
	RespondRefusal(w, http.StatusInternalServerError, Refusal{Message: "internal error", Code: "internal"})
}

// The lengths gouncer enforces, reported as data and pinned by TestRefusalLimitsMatchGouncer.
const (
	maxNameLength     = 256
	minPasswordLength = 12
	maxPasswordLength = 1024
)

// authRefusals maps each gouncer error a client may meet to its status and refusal.
var authRefusals = []struct {
	err     error
	status  int
	refusal Refusal
}{
	{gouncer.ErrInvalidEmail, http.StatusUnprocessableEntity,
		Refusal{Message: "invalid email address", Code: "email_invalid"}},
	{gouncer.ErrEmptyName, http.StatusUnprocessableEntity,
		Refusal{Message: "name is required", Code: "name_required"}},
	{gouncer.ErrNameTooLong, http.StatusUnprocessableEntity,
		Refusal{Message: "name must be at most 256 characters", Code: "name_too_long",
			Meta: map[string]any{"max": maxNameLength}}},
	{gouncer.ErrWeakPassword, http.StatusUnprocessableEntity,
		Refusal{Message: "password must be at least 12 characters", Code: "password_too_short",
			Meta: map[string]any{"min": minPasswordLength}}},
	{gouncer.ErrPasswordTooLong, http.StatusUnprocessableEntity,
		Refusal{Message: "password must be at most 1024 characters", Code: "password_too_long",
			Meta: map[string]any{"max": maxPasswordLength}}},
	{gouncer.ErrUserNotFound, http.StatusNotFound,
		Refusal{Message: "user not found", Code: "user_not_found"}},
	{gouncer.ErrEmailTaken, http.StatusConflict,
		Refusal{Message: "email already in use", Code: "email_taken"}},
}

// RefusalForAuthError returns the HTTP status code and named refusal for a
// gouncer error, reporting false for errors it does not recognize.
func RefusalForAuthError(err error) (int, Refusal, bool) {
	for _, held := range authRefusals {
		if errors.Is(err, held.err) {
			return held.status, held.refusal, true
		}
	}
	return 0, Refusal{}, false
}

// StatusForAuthError returns the HTTP status code and client-facing message
// for a gouncer error, reporting false for errors it does not recognize.
func StatusForAuthError(err error) (int, string, bool) {
	status, refusal, ok := RefusalForAuthError(err)
	return status, refusal.Message, ok
}

// MaxRequestBodyBytes caps how much of a request body Decode will read,
// so an unauthenticated caller cannot exhaust memory.
const MaxRequestBodyBytes = 1 << 20

// ErrBodyTooLarge reports that a request body exceeds [MaxRequestBodyBytes].
var ErrBodyTooLarge = errors.New("authkit: request body too large")

// Decode reads and JSON-decodes a single request body into a value of
// type T, bounding the body size and rejecting trailing content.
func Decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var v T
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&v); err != nil {
		var capped *http.MaxBytesError
		if errors.As(err, &capped) {
			return v, fmt.Errorf("decode json: %w", ErrBodyTooLarge)
		}
		return v, fmt.Errorf("decode json: %w", err)
	}
	if dec.More() {
		return v, errors.New("decode json: unexpected trailing content")
	}
	return v, nil
}

// respondDecodeError writes the refusal a failed request decode carries.
func respondDecodeError(w http.ResponseWriter, err error, message string) {
	if errors.Is(err, ErrBodyTooLarge) {
		RespondRefusal(w, http.StatusRequestEntityTooLarge, Refusal{
			Message: "request body too large", Code: "body_too_large",
			Meta: map[string]any{"max": MaxRequestBodyBytes},
		})
		return
	}
	RespondRefusal(w, http.StatusBadRequest, Refusal{Message: message, Code: "body_malformed"})
}
