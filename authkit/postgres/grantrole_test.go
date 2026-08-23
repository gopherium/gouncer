// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
)

func TestRunCreateAdminStartsTheAccountUnderTheRoleItIsGiven(t *testing.T) {
	t.Parallel()

	databaseURL := emptyDatabaseURL(t)
	var stdout strings.Builder

	err := authkitpg.RunCreateAdmin(
		t.Context(),
		databaseURL,
		[]string{"-email", "maria@example.com", "-name", "Maria Perez", "-role", "admin"},
		strings.NewReader("correct horse battery\n"),
		&stdout,
	)
	if err != nil {
		t.Fatalf("RunCreateAdmin() error = %v, want nil", err)
	}

	store := authkitpg.NewUserStore(newPoolFor(t, databaseURL))
	held, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if held.Role != "admin" {
		t.Errorf("role = %q, want %q", held.Role, "admin")
	}
}

func TestRunGrantRoleReachesEveryAccountHoldingNone(t *testing.T) {
	t.Parallel()

	databaseURL := emptyDatabaseURL(t)
	pool := newPoolFor(t, databaseURL)
	if err := authkitpg.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	store := authkitpg.NewUserStore(pool)
	ada := addWithRole(t, store, "ada@example.com", "")
	grace := addWithRole(t, store, "grace@example.com", "editor")
	var stdout strings.Builder

	err := authkitpg.RunGrantRole(t.Context(), databaseURL, []string{"-role", "admin"}, &stdout)
	if err != nil {
		t.Fatalf("RunGrantRole() error = %v, want nil", err)
	}

	if held, _ := store.UserByID(t.Context(), ada.ID); held.Role != "admin" {
		t.Errorf("account holding no role = %q, want %q", held.Role, "admin")
	}
	if kept, _ := store.UserByID(t.Context(), grace.ID); kept.Role != "editor" {
		t.Errorf("account with a role = %q, want it left at %q", kept.Role, "editor")
	}
	if stdout.String() != "granted admin to 1 account\n" {
		t.Errorf("stdout = %q, want it to report the one account that took the role", stdout.String())
	}

	var again strings.Builder
	if err := authkitpg.RunGrantRole(t.Context(), databaseURL, []string{"-role", "admin"}, &again); err != nil {
		t.Fatalf("second RunGrantRole() error = %v, want nil", err)
	}
	if again.String() != "granted admin to 0 accounts\n" {
		t.Errorf("second run said %q, want it to report granting nothing", again.String())
	}
	if held, _ := store.UserByID(t.Context(), ada.ID); held.Role != "admin" {
		t.Errorf("after a second run the granted account = %q, want %q", held.Role, "admin")
	}
	if kept, _ := store.UserByID(t.Context(), grace.ID); kept.Role != "editor" {
		t.Errorf("after a second run the account with a role = %q, want %q", kept.Role, "editor")
	}
}

func TestRunGrantRoleRefusesAnEmptyRole(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder

	err := authkitpg.RunGrantRole(t.Context(), emptyDatabaseURL(t), []string{"-role", ""}, &stdout)

	if !errors.Is(err, gouncer.ErrEmptyRole) {
		t.Errorf("RunGrantRole() error = %v, want ErrEmptyRole", err)
	}
}

func TestRunGrantRoleRefusesWithoutADatabase(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder

	err := authkitpg.RunGrantRole(t.Context(), "", []string{"-role", "admin"}, &stdout)

	if err == nil {
		t.Error("RunGrantRole() error = nil, want an error naming the missing database")
	}
}

// newPoolFor returns a pool over the database the url names.
func newPoolFor(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestRunCreateAdminRefusesAnEmptyRoleBeforeTouchingTheDatabase(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder

	err := authkitpg.RunCreateAdmin(
		t.Context(),
		unreachableDatabaseURL,
		[]string{"-email", "maria@example.com", "-name", "Maria Perez"},
		strings.NewReader("correct horse battery\n"),
		&stdout,
	)

	if !errors.Is(err, gouncer.ErrEmptyRole) {
		t.Errorf("RunCreateAdmin() error = %v, want ErrEmptyRole ahead of any connection", err)
	}
}
