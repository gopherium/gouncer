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

func TestCreateAdminStartsTheAccountUnderTheRankItIsGiven(t *testing.T) {
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
	if held.Rank != "admin" {
		t.Errorf("rank = %q, want %q, a bootstrap that ranks nobody leaves nobody able to administer", held.Rank, "admin")
	}
}

func TestCreateAdminRefusesAnEmptyRank(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	var out bytes.Buffer

	err := authkit.CreateAdmin(t.Context(), store, "maria@example.com", "Maria Perez", "",
		strings.NewReader(testPassword+"\n"), &out)

	if !errors.Is(err, gouncer.ErrEmptyRank) {
		t.Fatalf("CreateAdmin() error = %v, want ErrEmptyRank", err)
	}
	if _, err := store.UserByEmail(t.Context(), "maria@example.com"); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Error("the account was created, want the refused bootstrap to create nothing")
	}
}

func TestEnsureAdminStartsTheAccountUnderTheRankItIsGiven(t *testing.T) {
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
	if held.Rank != "admin" {
		t.Errorf("rank = %q, want %q", held.Rank, "admin")
	}
}

func TestEnsureAdminRefusesAnEmptyRank(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()

	_, err := authkit.EnsureAdmin(t.Context(), store, "maria@example.com", "Maria Perez", testPassword, "")

	if !errors.Is(err, gouncer.ErrEmptyRank) {
		t.Errorf("EnsureAdmin() error = %v, want ErrEmptyRank", err)
	}
}

func TestEnsureAdminLeavesAnAccountItAlreadyHolds(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	existing := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)
	existing.Rank = "editor"
	store.Users[existing.ID] = existing

	created, err := authkit.EnsureAdmin(t.Context(), store, "maria@example.com", "Maria Perez", testPassword, "admin")
	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}

	if created {
		t.Error("created = true, want false when the email is already taken")
	}
	if held, _ := store.UserByEmail(t.Context(), "maria@example.com"); held.Rank != "editor" {
		t.Errorf("rank = %q, want the rank the account already held", held.Rank)
	}
}
