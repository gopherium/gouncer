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

// RunGrantRank gives a rank to every account holding none, from command-line arguments.
func RunGrantRank(ctx context.Context, databaseURL string, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("grantrank", flag.ContinueOnError)
	flags.SetOutput(stdout)
	rank := flags.String("rank", "", "rank to give every account holding none")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("postgres: parse flags: %w", err)
	}

	if *rank == "" {
		return gouncer.ErrEmptyRank
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

	granted, err := NewUserStore(pool).GrantRankToRankless(ctx, *rank)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "granted %s to %d accounts\n", *rank, granted)
	return nil
}
