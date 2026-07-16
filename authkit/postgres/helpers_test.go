// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/postgres/testdb"
)

var (
	_ gouncer.Store         = (*postgres.UserStore)(nil)
	_ authkit.AdminStore    = (*postgres.UserStore)(nil)
	_ authkit.SessionReaper = (*postgres.UserStore)(nil)
)

// newTestPool returns a pgx pool on a fresh migrated test database.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	cfg := pgtestdb.Custom(t, testdb.Config(), testdb.Migrator())
	pool, err := pgxpool.New(t.Context(), cfg.URL())
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newTestDB returns a database handle on a fresh migrated test database.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	return pgtestdb.New(t, testdb.Config(), testdb.Migrator())
}
