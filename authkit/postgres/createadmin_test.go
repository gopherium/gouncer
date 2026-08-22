// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/postgres/testdb"
)

const unreachableDatabaseURL = "postgres://postgres:postgres@localhost:9/postgres?sslmode=disable&connect_timeout=1"

// emptyDatabaseURL returns the URL of a fresh unmigrated test database.
func emptyDatabaseURL(t *testing.T) string {
	t.Helper()
	return pgtestdb.Custom(t, testdb.Config(), pgtestdb.NoopMigrator{}).URL()
}

func TestRunCreateAdminProvisionsAUser(t *testing.T) {
	t.Parallel()

	databaseURL := emptyDatabaseURL(t)
	var stdout strings.Builder

	err := authkitpg.RunCreateAdmin(
		t.Context(),
		databaseURL,
		[]string{"-email", " Admin@Example.com ", "-name", "Admin", "-role", "admin"},
		strings.NewReader("correct horse battery\n"),
		&stdout,
	)

	if err != nil {
		t.Fatalf("RunCreateAdmin() error = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "admin@example.com") {
		t.Errorf("output = %q, want it to name the created user", stdout.String())
	}

	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	created, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the created user", err)
	}
	if !gouncer.VerifyPassword(created.PasswordHash, "correct horse battery") {
		t.Error("stored password hash does not verify against the entered password")
	}
}

func TestRunCreateAdminRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()

	databaseURL := emptyDatabaseURL(t)
	args := []string{"-email", "admin@example.com", "-name", "Admin", "-role", "admin"}

	if err := authkitpg.RunCreateAdmin(
		t.Context(), databaseURL, args, strings.NewReader("correct horse battery\n"), io.Discard,
	); err != nil {
		t.Fatalf("first RunCreateAdmin() error = %v, want nil", err)
	}

	err := authkitpg.RunCreateAdmin(
		t.Context(), databaseURL, args, strings.NewReader("correct horse battery\n"), io.Discard,
	)

	if !errors.Is(err, gouncer.ErrEmailTaken) {
		t.Errorf("RunCreateAdmin() error = %v, want %v", err, gouncer.ErrEmailTaken)
	}
}

func TestRunCreateAdminValidatesItsInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		databaseURL string
		args        []string
	}{
		"missing database url": {
			databaseURL: "",
			args:        []string{"-email", "admin@example.com", "-name", "Admin", "-role", "admin"},
		},
		"unknown flag": {
			databaseURL: "postgres://localhost/db",
			args:        []string{"-bogus"},
		},
		"malformed database url": {
			databaseURL: "not a url \x00",
			args:        []string{"-email", "admin@example.com", "-name", "Admin", "-role", "admin"},
		},
		"unreachable database": {
			databaseURL: unreachableDatabaseURL,
			args:        []string{"-email", "admin@example.com", "-name", "Admin", "-role", "admin"},
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			err := authkitpg.RunCreateAdmin(
				t.Context(), tc.databaseURL, tc.args, strings.NewReader("correct horse battery\n"), io.Discard,
			)

			if err == nil {
				t.Fatal("RunCreateAdmin() error = nil, want a failure")
			}
		})
	}
}
