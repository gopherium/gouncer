// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gopherium/gouncer"
)

// ErrorResponse is the JSON error body naming its condition as a stable code beside the message.
type ErrorResponse struct {
	Message string         `json:"error"`
	Code    string         `json:"code,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
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

// RespondError writes the JSON error body with the given status code.
func RespondError(w http.ResponseWriter, status int, response ErrorResponse) {
	Respond(w, status, response)
}

// respondAuthError writes the mapped auth error, masking unrecognized errors as internal ones.
func respondAuthError(w http.ResponseWriter, err error) {
	if status, response, ok := ErrorResponseForAuthError(err); ok {
		RespondError(w, status, response)
		return
	}
	RespondError(w, http.StatusInternalServerError, ErrorResponse{Message: "internal error", Code: "internal"})
}

// The lengths gouncer enforces, reported as data and pinned by TestErrorLimitsMatchGouncer.
const (
	maxNameLength     = 256
	minPasswordLength = 12
	maxPasswordLength = 1024
)

// authErrors maps each gouncer error a client may meet to its status and error body.
var authErrors = []struct {
	err      error
	status   int
	response ErrorResponse
}{
	{gouncer.ErrInvalidEmail, http.StatusUnprocessableEntity,
		ErrorResponse{Message: "invalid email address", Code: "email_invalid"}},
	{gouncer.ErrEmptyName, http.StatusUnprocessableEntity,
		ErrorResponse{Message: "name is required", Code: "name_required"}},
	{gouncer.ErrNameTooLong, http.StatusUnprocessableEntity,
		ErrorResponse{Message: "name must be at most 256 characters", Code: "name_too_long",
			Meta: map[string]any{"max": maxNameLength}}},
	{gouncer.ErrWeakPassword, http.StatusUnprocessableEntity,
		ErrorResponse{Message: "password must be at least 12 characters", Code: "password_too_short",
			Meta: map[string]any{"min": minPasswordLength}}},
	{gouncer.ErrPasswordTooLong, http.StatusUnprocessableEntity,
		ErrorResponse{Message: "password must be at most 1024 characters", Code: "password_too_long",
			Meta: map[string]any{"max": maxPasswordLength}}},
	{gouncer.ErrUserNotFound, http.StatusNotFound,
		ErrorResponse{Message: "user not found", Code: "user_not_found"}},
	{gouncer.ErrEmailTaken, http.StatusConflict,
		ErrorResponse{Message: "email already in use", Code: "email_taken"}},
}

// ErrorResponseForAuthError returns the HTTP status code and error body for a
// gouncer error, reporting false for errors it does not recognize.
func ErrorResponseForAuthError(err error) (int, ErrorResponse, bool) {
	for _, held := range authErrors {
		if errors.Is(err, held.err) {
			return held.status, held.response, true
		}
	}
	return 0, ErrorResponse{}, false
}

// StatusForAuthError returns the HTTP status code and client-facing message
// for a gouncer error, reporting false for errors it does not recognize.
func StatusForAuthError(err error) (int, string, bool) {
	status, response, ok := ErrorResponseForAuthError(err)
	return status, response.Message, ok
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

// respondDecodeError writes the error body a failed request decode carries.
func respondDecodeError(w http.ResponseWriter, err error, message string) {
	if errors.Is(err, ErrBodyTooLarge) {
		RespondError(w, http.StatusRequestEntityTooLarge, ErrorResponse{
			Message: "request body too large", Code: "body_too_large",
			Meta: map[string]any{"max": MaxRequestBodyBytes},
		})
		return
	}
	RespondError(w, http.StatusBadRequest, ErrorResponse{Message: message, Code: "body_malformed"})
}
