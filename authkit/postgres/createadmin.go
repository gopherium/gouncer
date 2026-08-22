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
	"github.com/gopherium/gouncer/authkit"
)

// RunCreateAdmin provisions a user account from command-line arguments,
// reading the password as one line from stdin.
func RunCreateAdmin(ctx context.Context, databaseURL string, args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("createadmin", flag.ContinueOnError)
	flags.SetOutput(stdout)
	email := flags.String("email", "", "email address of the new user")
	name := flags.String("name", "", "display name of the new user")
	role := flags.String("role", "", "role the new user starts under")
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

	return authkit.CreateAdmin(ctx, NewUserStore(pool), *email, *name, *role, stdin, stdout)
}
