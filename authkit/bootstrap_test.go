// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestCreateAdminStartsTheAccountUnderTheRoleItIsGiven(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	var out bytes.Buffer

	err := authkit.CreateAdmin(t.Context(), store, "maria@example.com", "Maria Perez", "admin",
		strings.NewReader(testPassword+"\n"), &out)
	if err != nil {
		t.Fatalf("CreateAdmin() error = %v, want nil", err)
	}

	held, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if held.Role != "admin" {
		t.Errorf("role = %q, want %q, a bootstrap giving nobody a role leaves nobody able to administer",
			held.Role, "admin")
	}
}

func TestCreateAdminRefusesAnEmptyRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	var out bytes.Buffer

	err := authkit.CreateAdmin(t.Context(), store, "maria@example.com", "Maria Perez", "",
		strings.NewReader(testPassword+"\n"), &out)

	if !errors.Is(err, gouncer.ErrEmptyRole) {
		t.Fatalf("CreateAdmin() error = %v, want ErrEmptyRole", err)
	}
	if _, err := store.UserByEmail(t.Context(), "maria@example.com"); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Error("the account was created, want the refused bootstrap to create nothing")
	}
}

func TestEnsureAdminStartsTheAccountUnderTheRoleItIsGiven(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()

	created, err := authkit.EnsureAdmin(t.Context(), store, "maria@example.com", "Maria Perez", testPassword, "admin")
	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}
	if !created {
		t.Fatal("created = false, want true on a store holding no such account")
	}

	held, _ := store.UserByEmail(t.Context(), "maria@example.com")
	if held.Role != "admin" {
		t.Errorf("role = %q, want %q", held.Role, "admin")
	}
}

func TestEnsureAdminRefusesAnEmptyRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()

	_, err := authkit.EnsureAdmin(t.Context(), store, "maria@example.com", "Maria Perez", testPassword, "")

	if !errors.Is(err, gouncer.ErrEmptyRole) {
		t.Errorf("EnsureAdmin() error = %v, want ErrEmptyRole", err)
	}
}

func TestEnsureAdminLeavesAnAccountItAlreadyHolds(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	existing := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	existing.Role = "editor"
	store.Users[existing.ID] = existing

	created, err := authkit.EnsureAdmin(t.Context(), store, "maria@example.com", "Maria Perez", testPassword, "admin")
	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}

	if created {
		t.Error("created = true, want false when the email is already taken")
	}
	if held, _ := store.UserByEmail(t.Context(), "maria@example.com"); held.Role != "editor" {
		t.Errorf("role = %q, want the role the account already held", held.Role)
	}
}
