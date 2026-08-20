// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/gouncer/authkit/postgres/internal/db"
)

// holdPrivileged locks every account holding a privileged rank, in a settled order.
const holdPrivileged = `SELECT id FROM auth.users WHERE rank = ANY($1) ORDER BY id FOR UPDATE`

// countCover counts the enabled accounts holding a privileged rank, apart from one.
const countCover = `SELECT count(*) FROM auth.users WHERE rank = ANY($1) AND NOT disabled AND id <> $2`

// SetUserRank writes an account's rank, refusing to leave no enabled account under a privileged one.
func (s *UserStore) SetUserRank(
	ctx context.Context,
	id uuid.UUID,
	rank string,
	privileged gouncer.Ranks,
) error {
	return s.underCover(ctx, id, privileged, !privileged.Holds(rank), func(queries *db.Queries) error {
		count, err := queries.SetUserRank(ctx, db.SetUserRankParams{ID: id, Rank: rank})
		if err != nil {
			return fmt.Errorf("postgres: set user rank: %w", err)
		}
		if count == 0 {
			return gouncer.ErrUserNotFound
		}
		return nil
	})
}

// SetUserDisabledUnderCover disables an account, refusing to leave no enabled account privileged.
func (s *UserStore) SetUserDisabledUnderCover(
	ctx context.Context,
	id uuid.UUID,
	disabled bool,
	privileged gouncer.Ranks,
) error {
	return s.underCover(ctx, id, privileged, disabled, func(queries *db.Queries) error {
		return s.disable(ctx, queries, id, disabled)
	})
}

// underCover runs a write in one transaction, refusing it when it would remove the last cover.
func (s *UserStore) underCover(
	ctx context.Context,
	id uuid.UUID,
	privileged gouncer.Ranks,
	removesCover bool,
	write func(*db.Queries) error,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.refuseUncovered(ctx, tx, id, privileged, removesCover); err != nil {
		return err
	}
	if err := write(s.queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	return nil
}

// refuseUncovered holds the privileged accounts and refuses a write leaving none enabled.
func (s *UserStore) refuseUncovered(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
	privileged gouncer.Ranks,
	removesCover bool,
) error {
	if len(privileged) == 0 || !removesCover {
		return nil
	}
	if _, err := tx.Exec(ctx, holdPrivileged, []string(privileged)); err != nil {
		return fmt.Errorf("postgres: hold privileged: %w", err)
	}
	held, err := s.holdsPrivilege(ctx, tx, id, privileged)
	if err != nil || !held {
		return err
	}
	var cover int
	if err := tx.QueryRow(ctx, countCover, []string(privileged), id).Scan(&cover); err != nil {
		return fmt.Errorf("postgres: count cover: %w", err)
	}
	if cover == 0 {
		return gouncer.ErrLastPrivileged
	}
	return nil
}

// holdsPrivilege reports whether an enabled account currently holds a privileged rank.
func (s *UserStore) holdsPrivilege(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
	privileged gouncer.Ranks,
) (bool, error) {
	const query = `SELECT count(*) FROM auth.users WHERE id = $1 AND rank = ANY($2) AND NOT disabled`
	var held int
	if err := tx.QueryRow(ctx, query, id, []string(privileged)).Scan(&held); err != nil {
		return false, fmt.Errorf("postgres: read rank: %w", err)
	}
	return held > 0, nil
}

// disable writes an account's disabled state and sweeps its sessions when it closes.
func (s *UserStore) disable(ctx context.Context, queries *db.Queries, id uuid.UUID, disabled bool) error {
	count, err := queries.SetUserDisabled(ctx, db.SetUserDisabledParams{ID: id, Disabled: disabled})
	if err != nil {
		return fmt.Errorf("postgres: set user disabled: %w", err)
	}
	if count == 0 {
		return gouncer.ErrUserNotFound
	}
	if disabled {
		if err := queries.DeleteUserSessions(ctx, id); err != nil {
			return fmt.Errorf("postgres: revoke user sessions: %w", err)
		}
	}
	return nil
}

// GrantRankToRankless gives a rank to every account holding none, reporting how many took it.
func (s *UserStore) GrantRankToRankless(ctx context.Context, rank string) (int64, error) {
	if rank == "" {
		return 0, gouncer.ErrEmptyRank
	}
	granted, err := s.queries.GrantRankToRankless(ctx, rank)
	if err != nil {
		return 0, fmt.Errorf("postgres: grant rank: %w", err)
	}
	return granted, nil
}
