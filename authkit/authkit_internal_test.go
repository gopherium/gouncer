// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

type stubUserStore struct {
	byEmail gouncer.User
}

func (s stubUserStore) CreateUser(context.Context, gouncer.User) error {
	return nil
}

func (s stubUserStore) UserByEmail(context.Context, string) (gouncer.User, error) {
	return s.byEmail, nil
}

func (s stubUserStore) CreateSession(context.Context, gouncer.Session) error {
	return nil
}

func (s stubUserStore) UserBySession(context.Context, []byte, time.Time) (gouncer.User, error) {
	return gouncer.User{}, gouncer.ErrSessionNotFound
}

func (s stubUserStore) DeleteSession(context.Context, []byte) error {
	return nil
}

func TestLoginReportsSessionIssuanceFailure(t *testing.T) {
	t.Parallel()

	account, err := gouncer.NewUser("known@example.com", "Known", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	h := New(Config{Store: stubUserStore{byEmail: account}})
	h.newSession = func(uuid.UUID) (gouncer.Session, error) {
		return gouncer.Session{}, errors.New("no entropy")
	}
	body := `{"email":"known@example.com","password":"correct horse battery"}`
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	recorder := httptest.NewRecorder()

	h.Login(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestRespondMasksMarshalFailures(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()

	Respond(recorder, http.StatusOK, func() {})

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if got := recorder.Body.String(); got != `{"error":"internal error"}` {
		t.Errorf("body = %q, want a masked internal error", got)
	}
}
