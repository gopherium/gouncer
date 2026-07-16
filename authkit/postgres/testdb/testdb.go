// SPDX-License-Identifier: Apache-2.0

// Package testdb provides pgtestdb wiring for tests against the module's
// auth schema.
package testdb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"

	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/gouncer/authkit/postgres"
)

// Config returns the pgtestdb configuration for the module's local
// compose database.
func Config() pgtestdb.Config {
	return pgtestdb.Config{
		DriverName: "pgx",
		User:       "postgres",
		Password:   "postgres",
		Host:       "localhost",
		Port:       "5434",
		Database:   "postgres",
		Options:    "sslmode=disable",
	}
}

// Migrator returns a pgtestdb migrator applying the module's migrations
// through [postgres.Migrate], so test databases match production ones.
func Migrator() pgtestdb.Migrator {
	return migrator{}
}

type migrator struct{}

// Hash fingerprints the embedded migration files.
func (migrator) Hash() (string, error) {
	h := sha256.New()
	err := fs.WalkDir(postgres.Migrations, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(postgres.Migrations, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprint(h, path)
		_, _ = h.Write(content)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("testdb: hash migrations: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Prepare does nothing before migrating.
func (migrator) Prepare(context.Context, *sql.DB, pgtestdb.Config) error {
	return nil
}

// Migrate applies the module's migrations to the template database.
func (migrator) Migrate(ctx context.Context, _ *sql.DB, cfg pgtestdb.Config) error {
	return postgres.Migrate(ctx, cfg.URL())
}

// Verify accepts any migrated database.
func (migrator) Verify(context.Context, *sql.DB, pgtestdb.Config) error {
	return nil
}
