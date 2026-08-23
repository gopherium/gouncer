// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

// errNotAnAuthError stands for a failure the auth mapping does not recognize.
var errNotAnAuthError = errors.New("authkit: not an auth error")

// decodeError reads the recorded error response.
func decodeError(t *testing.T, recorder *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the error body: %v, want nil", err)
	}
	return body
}

func TestRespondErrorCarriesNoCode(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	authkit.RespondError(recorder, http.StatusBadRequest, authkit.ErrorResponse{Message: "malformed json"})

	if got := recorder.Body.String(); got != `{"error":"malformed json"}` {
		t.Errorf("body = %s, want the shape a pinned client already reads", got)
	}
}

func TestRespondErrorCarriesTheCodeBesideTheMessage(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	authkit.RespondError(recorder, http.StatusUnprocessableEntity, authkit.ErrorResponse{
		Message: "invalid email address",
		Code:    "email_invalid",
	})

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	body := decodeError(t, recorder)
	if body.Error != "invalid email address" || body.Code != "email_invalid" {
		t.Errorf("body = %+v, want the message and the code together", body)
	}
}

func TestRespondErrorCarriesTheDynamicPartAsData(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	authkit.RespondError(recorder, http.StatusUnprocessableEntity, authkit.ErrorResponse{
		Message: "name must be at most 256 characters",
		Code:    "name_too_long",
		Meta:    map[string]any{"max": 256},
	})

	body := decodeError(t, recorder)
	if body.Meta["max"] != float64(256) {
		t.Errorf("meta = %v, want the limit as data rather than only in the prose", body.Meta)
	}
}

func TestRespondErrorLeavesAnEmptyCodeOffTheWire(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	authkit.RespondError(recorder, http.StatusNotFound, authkit.ErrorResponse{Message: "not found"})

	if got := recorder.Body.String(); got != `{"error":"not found"}` {
		t.Errorf("body = %s, want no empty members on the wire", got)
	}
}

func TestErrorResponseForAuthErrorNamesACode(t *testing.T) {
	t.Parallel()

	status, response, ok := authkit.ErrorResponseForAuthError(gouncer.ErrEmailTaken)

	if !ok {
		t.Fatalf("ErrorResponseForAuthError() ok = false, want the error recognized")
	}
	if status != http.StatusConflict {
		t.Errorf("status = %d, want %d", status, http.StatusConflict)
	}
	if response.Code != "email_taken" {
		t.Errorf("code = %q, want the taken email named", response.Code)
	}
	if response.Message != "email already in use" {
		t.Errorf("message = %q, want the prose kept beside the code", response.Message)
	}
}

func TestErrorResponseForAuthErrorReportsAnUnknownError(t *testing.T) {
	t.Parallel()

	if _, _, ok := authkit.ErrorResponseForAuthError(errNotAnAuthError); ok {
		t.Errorf("ErrorResponseForAuthError() ok = true, want an unrecognized error reported")
	}
}

func TestDecodeReportsABodyBeyondTheCap(t *testing.T) {
	t.Parallel()

	body := strings.NewReader(`{"pad":"` + strings.Repeat("a", authkit.MaxRequestBodyBytes) + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/", body)
	recorder := httptest.NewRecorder()

	_, err := authkit.Decode[map[string]string](recorder, request)

	if !errors.Is(err, authkit.ErrBodyTooLarge) {
		t.Fatalf("Decode() error = %v, want %v", err, authkit.ErrBodyTooLarge)
	}
}

func TestLoginRefusesAnOversizedBodyWithItsCode(t *testing.T) {
	t.Parallel()

	handler := authkit.New(authkit.Config{Store: testkit.NewStore()})
	body := strings.NewReader(`{"email":"` + strings.Repeat("a", authkit.MaxRequestBodyBytes) + `"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	recorder := httptest.NewRecorder()

	handler.Login(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
	held := decodeError(t, recorder)
	if held.Code != "body_too_large" {
		t.Errorf("code = %q, want the cap named rather than a malformed body", held.Code)
	}
	if held.Meta["max"] != float64(authkit.MaxRequestBodyBytes) {
		t.Errorf("meta = %v, want the cap carried as data", held.Meta)
	}
}

func TestDecodeStillReportsMalformedJSON(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"pad":`))
	recorder := httptest.NewRecorder()

	_, err := authkit.Decode[map[string]string](recorder, request)

	if err == nil || errors.Is(err, authkit.ErrBodyTooLarge) {
		t.Fatalf("Decode() error = %v, want a plain decode failure", err)
	}
}

func TestStatusForAuthErrorKeepsItsAnswer(t *testing.T) {
	t.Parallel()

	status, message, ok := authkit.StatusForAuthError(gouncer.ErrEmailTaken)

	if !ok || status != http.StatusConflict || message != "email already in use" {
		t.Errorf("StatusForAuthError() = %d, %q, %v, want the pinned answer unchanged", status, message, ok)
	}
}
