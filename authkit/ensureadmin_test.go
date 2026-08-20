// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"errors"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestEnsureAdminCreatesTheAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()

	created, err := authkit.EnsureAdmin(t.Context(), store, " Admin@Example.com ", "Admin", "password1234", "admin")

	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}
	if !created {
		t.Error("EnsureAdmin() created = false, want true")
	}
	stored, err := store.UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the created user", err)
	}
	if !gouncer.VerifyPassword(stored.PasswordHash, "password1234") {
		t.Error("stored password hash does not verify against the given password")
	}
}

func TestEnsureAdminKeepsAnExistingAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	_, err := authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "first password", "admin")
	if err != nil {
		t.Fatalf("first EnsureAdmin() error = %v, want nil", err)
	}

	created, err := authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "second password", "admin")

	if err != nil {
		t.Fatalf("second EnsureAdmin() error = %v, want nil", err)
	}
	if created {
		t.Error("second EnsureAdmin() created = true, want false")
	}
	stored, err := store.UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the existing user", err)
	}
	if !gouncer.VerifyPassword(stored.PasswordHash, "first password") {
		t.Error("existing password hash changed, want it untouched")
	}
}

func TestEnsureAdminRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		email    string
		password string
	}{
		"weak password": {email: "admin@example.com", password: "short"},
		"invalid email": {email: "not an email", password: "password1234"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			created, err := authkit.EnsureAdmin(t.Context(), testkit.NewStore(), tc.email, "Admin", tc.password, "admin")

			if err == nil {
				t.Fatal("EnsureAdmin() error = nil, want a validation failure")
			}
			if created {
				t.Error("EnsureAdmin() created = true, want false")
			}
		})
	}
}

func TestEnsureAdminReportsStoreFailure(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.CreateUserErr = errors.New("store down")

	created, err := authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "password1234", "admin")

	if !errors.Is(err, store.CreateUserErr) {
		t.Fatalf("EnsureAdmin() error = %v, want the store failure", err)
	}
	if created {
		t.Error("EnsureAdmin() created = true, want false")
	}
}
