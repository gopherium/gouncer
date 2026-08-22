// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestSessionIdentityCarriesTheRoleTheStoreHolds(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	ada := addAda(t, store)
	ada.Role = "admin"
	store.Users[ada.ID] = ada
	handlers := authkit.New(authkit.Config{Store: store})
	session, err := gouncer.NewSession(ada.ID)
	if err != nil {
		t.Fatalf("NewSession() error = %v, want nil", err)
	}
	session.ExpiresAt = session.CreatedAt.Add(time.Hour)
	if err := store.CreateSession(context.Background(), session); err != nil {
		t.Fatalf("CreateSession() error = %v, want nil", err)
	}

	identity, err := handlers.SessionIdentity(context.Background(), session.Token)
	if err != nil {
		t.Fatalf("SessionIdentity() error = %v, want nil", err)
	}

	if identity.Role != "admin" {
		t.Errorf("role = %q, want %q", identity.Role, "admin")
	}
}

func TestAuthenticateCarriesTheRoleTheStoreHolds(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	ada := addAda(t, store)
	ada.Role = "editor"
	store.Users[ada.ID] = ada
	handlers := authkit.New(authkit.Config{Store: store})

	identity, err := handlers.Authenticate(context.Background(), "ada@example.com", testPassword)
	if err != nil {
		t.Fatalf("Authenticate() error = %v, want nil", err)
	}

	if identity.Role != "editor" {
		t.Errorf("role = %q, want %q", identity.Role, "editor")
	}
}

func TestRequirePrivilegeRefusesARoleOutsideTheCover(t *testing.T) {
	t.Parallel()

	handlers := authkit.New(authkit.Config{
		Store:      testkit.NewStore(),
		Privileged: gouncer.Roles{"admin"},
	})
	guarded := handlers.RequirePrivilege(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request = request.WithContext(authkit.WithIdentity(request.Context(), authkit.Identity{Role: "editor"}))
	recorder := httptest.NewRecorder()

	guarded.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if code := decodeBody[authkit.Refusal](t, recorder).Code; code != "role_insufficient" {
		t.Errorf("code = %q, want %q", code, "role_insufficient")
	}
}

func TestRequirePrivilegeAdmitsARoleInsideTheCover(t *testing.T) {
	t.Parallel()

	handlers := authkit.New(authkit.Config{
		Store:      testkit.NewStore(),
		Privileged: gouncer.Roles{"admin", "owner"},
	})
	guarded := handlers.RequirePrivilege(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request = request.WithContext(authkit.WithIdentity(request.Context(), authkit.Identity{Role: "owner"}))
	recorder := httptest.NewRecorder()

	guarded.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestRequirePrivilegeKeepsItsOwnCopyOfTheConfiguredRoles(t *testing.T) {
	t.Parallel()

	configured := gouncer.Roles{"admin"}
	handlers := authkit.New(authkit.Config{Store: testkit.NewStore(), Privileged: configured})
	configured[0] = "editor"
	guarded := handlers.RequirePrivilege(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request = request.WithContext(authkit.WithIdentity(request.Context(), authkit.Identity{Role: "editor"}))
	recorder := httptest.NewRecorder()

	guarded.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d, a caller mutating its own slice must not widen the gate",
			recorder.Code, http.StatusForbidden)
	}
}

func TestRequirePrivilegeAdmitsEveryoneWhenNoCoverIsConfigured(t *testing.T) {
	t.Parallel()

	handlers := authkit.New(authkit.Config{Store: testkit.NewStore()})
	guarded := handlers.RequirePrivilege(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	request = request.WithContext(authkit.WithIdentity(request.Context(), authkit.Identity{}))
	recorder := httptest.NewRecorder()

	guarded.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d when no privileged roles are configured", recorder.Code, http.StatusOK)
	}
}
