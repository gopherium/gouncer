// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/postgres/testdb"
)

// freshDatabaseURL returns the URL of an empty, unmigrated test database.
func freshDatabaseURL(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	return pgtestdb.Custom(t, testdb.Config(), pgtestdb.NoopMigrator{}).URL()
}

func TestMigrateCreatesAuthSchema(t *testing.T) {
	t.Parallel()

	databaseURL := freshDatabaseURL(t)

	if err := postgres.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	if err := postgres.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("second Migrate() error = %v, want idempotent nil", err)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer func() { _ = db.Close() }()
	ada, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	if _, err := db.Exec(
		"INSERT INTO auth.users (id, email, name, password_hash, disabled, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
		ada.ID, ada.Email, ada.Name, ada.PasswordHash, ada.Disabled, ada.CreatedAt,
	); err != nil {
		t.Fatalf("inserting into migrated schema: %v", err)
	}
}

func TestMigrateUsesItsOwnVersionTable(t *testing.T) {
	t.Parallel()

	databaseURL := freshDatabaseURL(t)
	if err := postgres.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer func() { _ = db.Close() }()
	var count int
	err = db.QueryRow(
		"SELECT count(*) FROM information_schema.tables WHERE table_schema = 'auth' AND table_name = 'goose_db_version'",
	).Scan(&count)

	if err != nil {
		t.Fatalf("looking up the version table: %v", err)
	}
	if count != 1 {
		t.Errorf("auth.goose_db_version tables = %d, want 1 (the module's own lineage)", count)
	}
}

func TestMigrationsIndexSessionsExpiresAt(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	var indexdef string
	err := db.QueryRow(
		"SELECT indexdef FROM pg_indexes WHERE schemaname = 'auth' AND indexname = 'sessions_expires_at_idx'",
	).Scan(&indexdef)

	if err != nil {
		t.Fatalf("looking up sessions_expires_at_idx: %v, want the index to exist", err)
	}
	if !strings.Contains(indexdef, "(expires_at)") {
		t.Errorf("indexdef = %q, want an index on (expires_at)", indexdef)
	}
}

func TestMigrateRejectsMalformedURL(t *testing.T) {
	t.Parallel()

	if err := postgres.Migrate(t.Context(), "://not-a-url"); err == nil {
		t.Fatal("Migrate() error = nil, want a parse error")
	}
}

func TestMigrateReportsUnreachableDatabase(t *testing.T) {
	t.Parallel()

	err := postgres.Migrate(
		t.Context(),
		"postgres://postgres:postgres@localhost:9/postgres?sslmode=disable&connect_timeout=1",
	)

	if err == nil {
		t.Fatal("Migrate() error = nil, want a connection error")
	}
}

func TestMigrateReportsFailedMigrations(t *testing.T) {
	t.Parallel()

	databaseURL := freshDatabaseURL(t)
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("CREATE SCHEMA auth; CREATE TABLE auth.users (id int)"); err != nil {
		t.Fatalf("planting the conflicting table: %v", err)
	}

	if err := postgres.Migrate(t.Context(), databaseURL); err == nil {
		t.Fatal("Migrate() error = nil, want a failed migration")
	}
}
