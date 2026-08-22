// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
)

// RunGrantRole gives a role to every account holding none, from command-line arguments.
func RunGrantRole(ctx context.Context, databaseURL string, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("grantrole", flag.ContinueOnError)
	flags.SetOutput(stdout)
	role := flags.String("role", "", "role to give every account holding none")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("postgres: parse flags: %w", err)
	}

	if *role == "" {
		return gouncer.ErrEmptyRole
	}
	if databaseURL == "" {
		return errors.New("postgres: database url is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("postgres: parse database url: %w", err)
	}
	defer pool.Close()
	if err := Migrate(ctx, databaseURL); err != nil {
		return err
	}

	granted, err := NewUserStore(pool).GrantRoleToRoleless(ctx, *role)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "granted %s to %d %s\n", *role, granted, accountNoun(granted))
	return nil
}

// accountNoun names the account noun matching the count.
func accountNoun(granted int64) string {
	if granted == 1 {
		return "account"
	}
	return "accounts"
}
