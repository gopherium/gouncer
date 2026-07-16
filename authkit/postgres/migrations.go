// SPDX-License-Identifier: Apache-2.0

// Package postgres implements gouncer's user and session persistence in
// a PostgreSQL schema of its own.
package postgres

import "embed"

// Migrations holds the auth schema migration files applied by goose.
//
//go:embed migrations/*.sql
var Migrations embed.FS
