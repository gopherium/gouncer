// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/postgres/internal/db"
)

// CreateToken stores t for an enabled account while fewer than live
// unexpired tokens stand for the same user and purpose, or returns
// [gouncer.ErrTokenExists] at the cap and [gouncer.ErrUserNotFound] for
// a disabled one.
func (s *UserStore) CreateToken(ctx context.Context, t gouncer.Token, live int) error {
	return s.inTx(ctx, "create token", func(queries *db.Queries) error {
		if err := holdEnabled(ctx, queries, t.UserID); err != nil {
			return err
		}
		standing, err := queries.CountLiveTokens(ctx, db.CountLiveTokensParams{
			UserID:    t.UserID,
			Purpose:   string(t.Purpose),
			ExpiresAt: time.Now().UTC(),
		})
		if err != nil {
			return err
		}
		if standing >= int64(live) {
			return gouncer.ErrTokenExists
		}
		return insertToken(ctx, queries, t)
	})
}

// ReplaceToken stores t for an enabled account in place of every token
// it holds for the same purpose, or returns [gouncer.ErrUserNotFound]
// for a disabled one and for an activated one under
// [gouncer.PurposeInvite], replacing nothing when it refuses.
func (s *UserStore) ReplaceToken(ctx context.Context, t gouncer.Token) error {
	return s.inTx(ctx, "replace token", func(queries *db.Queries) error {
		if err := holdReplaceable(ctx, queries, t.UserID, t.Purpose); err != nil {
			return err
		}
		err := queries.DeleteUserTokensForPurpose(ctx, db.DeleteUserTokensForPurposeParams{
			UserID:  t.UserID,
			Purpose: string(t.Purpose),
		})
		if err != nil {
			return err
		}
		return insertToken(ctx, queries, t)
	})
}

// ActivateByToken spends the invite token, storing the password hash and
// confirming the account it names, answering the account's id,
// [gouncer.ErrTokenNotFound] for a spent or expired token, and
// [gouncer.ErrUserNotFound] for a disabled or confirmed account.
func (s *UserStore) ActivateByToken(
	ctx context.Context,
	tokenHash []byte,
	now time.Time,
	passwordHash string,
) (uuid.UUID, error) {
	return s.redeem(ctx, "activate by token", tokenHash, gouncer.PurposeInvite, now,
		func(queries *db.Queries, id uuid.UUID) (int64, error) {
			return queries.ActivateUser(ctx, db.ActivateUserParams{ID: id, PasswordHash: passwordHash})
		})
}

// ResetByToken spends the reset token, storing the password hash and
// ending every session the account it names holds, answering the
// account's id, [gouncer.ErrTokenNotFound] for a spent or expired token,
// and [gouncer.ErrUserNotFound] for a disabled account.
func (s *UserStore) ResetByToken(
	ctx context.Context,
	tokenHash []byte,
	now time.Time,
	passwordHash string,
) (uuid.UUID, error) {
	return s.redeem(ctx, "reset by token", tokenHash, gouncer.PurposeReset, now,
		func(queries *db.Queries, id uuid.UUID) (int64, error) {
			count, err := queries.SetUserPassword(ctx, db.SetUserPasswordParams{ID: id, PasswordHash: passwordHash})
			if err != nil || count == 0 {
				return count, err
			}
			return count, queries.DeleteUserSessions(ctx, id)
		})
}

// DeleteExpiredTokens removes expired tokens and the unconfirmed
// accounts an expired invite leaves behind, reporting how many tokens went.
func (s *UserStore) DeleteExpiredTokens(ctx context.Context, now time.Time) (int64, error) {
	var swept int64
	err := s.inTx(ctx, "delete expired tokens", func(queries *db.Queries) error {
		expired, err := queries.ExpiredTokens(ctx, now)
		if err != nil {
			return err
		}
		swept = int64(len(expired))
		stranded := make([]uuid.UUID, 0, len(expired))
		for _, row := range expired {
			if row.Purpose == string(gouncer.PurposeInvite) {
				stranded = append(stranded, row.UserID)
			}
		}
		if err := sweepStranded(ctx, queries, stranded, now); err != nil {
			return err
		}
		_, err = queries.DeleteExpiredTokens(ctx, now)
		return err
	})
	if err != nil {
		return 0, err
	}
	return swept, nil
}

// redeem spends the live token behind the hash and purpose and applies
// write to the account it names, answering that account's id.
func (s *UserStore) redeem(
	ctx context.Context,
	action string,
	tokenHash []byte,
	purpose gouncer.TokenPurpose,
	now time.Time,
	write func(*db.Queries, uuid.UUID) (int64, error),
) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.inTx(ctx, action, func(queries *db.Queries) error {
		held, err := queries.FindLiveToken(ctx, db.FindLiveTokenParams{
			TokenHash: tokenHash,
			Purpose:   string(purpose),
			ExpiresAt: now,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return gouncer.ErrTokenNotFound
		}
		if err != nil {
			return err
		}
		if err := holdEnabled(ctx, queries, held); err != nil {
			return err
		}
		spent, err := queries.DeleteToken(ctx, tokenHash)
		if err != nil {
			return err
		}
		if spent == 0 {
			return gouncer.ErrTokenNotFound
		}
		count, err := write(queries, held)
		if err != nil {
			return err
		}
		if count == 0 {
			return gouncer.ErrUserNotFound
		}
		id = held
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// inTx runs body against a transaction, committing it or wrapping the failure under action.
func (s *UserStore) inTx(ctx context.Context, action string, body func(*db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: %s: %w", action, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := body(s.queries.WithTx(tx)); err != nil {
		if isSentinel(err) {
			return err
		}
		return fmt.Errorf("postgres: %s: %w", action, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: %s: %w", action, err)
	}
	return nil
}

// sweepStranded takes the accounts an expired invite left behind, holding
// each one first so a replacement committing beside the sweep is seen.
func sweepStranded(ctx context.Context, queries *db.Queries, stranded []uuid.UUID, now time.Time) error {
	if len(stranded) == 0 {
		return nil
	}
	held, err := queries.LockUnconfirmedAccounts(ctx, stranded)
	if err != nil || len(held) == 0 {
		return err
	}
	return queries.DeleteUnconfirmedAccounts(ctx, db.DeleteUnconfirmedAccountsParams{
		Column1:   held,
		Purpose:   string(gouncer.PurposeInvite),
		ExpiresAt: now,
	})
}

// holdReplaceable locks the account a replacement token may stand for,
// requiring an unactivated one under [gouncer.PurposeInvite].
func holdReplaceable(
	ctx context.Context,
	queries *db.Queries,
	id uuid.UUID,
	purpose gouncer.TokenPurpose,
) error {
	if purpose != gouncer.PurposeInvite {
		return holdEnabled(ctx, queries, id)
	}
	_, err := queries.LockUnactivatedUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return gouncer.ErrUserNotFound
	}
	return err
}

// holdEnabled locks the enabled account against a concurrent disable, or
// reports [gouncer.ErrUserNotFound].
func holdEnabled(ctx context.Context, queries *db.Queries, id uuid.UUID) error {
	_, err := queries.LockEnabledUser(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return gouncer.ErrUserNotFound
	}
	return err
}

// insertToken stores t for the account the caller already holds.
func insertToken(ctx context.Context, queries *db.Queries, t gouncer.Token) error {
	return queries.CreateToken(ctx, db.CreateTokenParams{
		TokenHash: t.TokenHash,
		UserID:    t.UserID,
		Purpose:   string(t.Purpose),
		CreatedAt: t.CreatedAt,
		ExpiresAt: t.ExpiresAt,
	})
}

// isSentinel answers whether err is a gouncer contract error the caller matches on.
func isSentinel(err error) bool {
	return errors.Is(err, gouncer.ErrTokenExists) ||
		errors.Is(err, gouncer.ErrTokenNotFound) ||
		errors.Is(err, gouncer.ErrUserNotFound)
}
