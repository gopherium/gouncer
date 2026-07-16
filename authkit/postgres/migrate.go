// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
)

var migrationSource = mustSub(Migrations, "migrations")

// versionTable keeps the module's migration lineage inside its own
// schema, away from any goose table the application owns.
const versionTable = "auth.goose_db_version"

// Migrate applies the auth schema migrations to the database at databaseURL.
func Migrate(ctx context.Context, databaseURL string) error {
	return migrateDatabase(ctx, "pgx", databaseURL)
}

// migrateDatabase opens a database connection using driverName and databaseURL and applies the migrations.
func migrateDatabase(ctx context.Context, driverName, databaseURL string) error {
	db, err := sql.Open(driverName, databaseURL)
	if err != nil {
		return fmt.Errorf("postgres: open database: %w", err)
	}
	defer func() { _ = db.Close() }()
	return migrate(ctx, db)
}

// migrate creates the auth schema and runs all pending up migrations
// against the module's own goose version table.
func migrate(ctx context.Context, db *sql.DB) error {
	store, err := database.NewStore(database.DialectPostgres, versionTable)
	if err != nil {
		return fmt.Errorf("postgres: migration store: %w", err)
	}
	provider, err := goose.NewProvider("", db, migrationSource, goose.WithStore(store))
	if err != nil {
		return fmt.Errorf("postgres: migration provider: %w", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS auth"); err != nil {
		return fmt.Errorf("postgres: create schema: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("postgres: apply migrations: %w", err)
	}
	return nil
}

// mustSub returns the dir subtree of fsys and panics if it cannot be created.
func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
