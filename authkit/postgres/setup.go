// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
)

// UserStore implements authkit.SetupStore.
var _ authkit.SetupStore = (*UserStore)(nil)

// lockUsers makes every other write on the accounts wait until the transaction ends, and lets reads go on.
const lockUsers = `LOCK TABLE auth.users IN SHARE ROW EXCLUSIVE MODE`

// isolationLevel reads the isolation level of the transaction it runs in.
const isolationLevel = `SELECT current_setting('transaction_isolation')`

// freshSnapshots is the isolation level under which each statement sees every write committed before it.
const freshSnapshots = "read committed"

// HasUsers reports whether the store holds any account, disabled and unconfirmed ones included.
func (s *UserStore) HasUsers(ctx context.Context) (bool, error) {
	held, err := s.queries.HasUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("postgres: has users: %w", err)
	}
	return held, nil
}

// CreateFirstUser stores u while the store holds no account, or answers authkit.ErrSetupDone and stores nothing.
func (s *UserStore) CreateFirstUser(ctx context.Context, u gouncer.User) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, lockUsers); err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	if err := seesLatest(ctx, tx); err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	queries := s.queries.WithTx(tx)
	held, err := queries.HasUsers(ctx)
	if err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	if held {
		return authkit.ErrSetupDone
	}
	if err := insertUser(ctx, queries, u); err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: create first user: %w", err)
	}
	return nil
}

// seesLatest answers an error unless each statement of tx sees every write committed before it.
func seesLatest(ctx context.Context, tx pgx.Tx) error {
	var level string
	if err := tx.QueryRow(ctx, isolationLevel).Scan(&level); err != nil {
		return err
	}
	if level != freshSnapshots {
		return fmt.Errorf("a first account needs READ COMMITTED, the transaction runs %s", level)
	}
	return nil
}
