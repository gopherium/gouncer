// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/postgres"
)

// openTx begins a transaction on the pool that rolls back when the test ends.
func openTx(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

// invitedWithin stores an unconfirmed account through the transaction scoped store.
func invitedWithin(t *testing.T, scoped *postgres.UserStore, email, name string) gouncer.User {
	t.Helper()
	u, err := gouncer.NewInvitedUser(email, name)
	if err != nil {
		t.Fatalf("gouncer.NewInvitedUser() error = %v, want nil", err)
	}
	if err := scoped.CreateUser(t.Context(), u); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return u
}

func TestWithinRollsTheUserBackWithTheCaller(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	tx := openTx(t, pool)

	invitedWithin(t, store.Within(tx), "maria@example.com", "Maria Perez")
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatalf("Rollback() error = %v, want nil", err)
	}

	if _, err := store.UserByEmail(t.Context(), "maria@example.com"); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Fatalf("UserByEmail() after rollback error = %v, want ErrUserNotFound", err)
	}
}

func TestWithinLandsTheUserWithTheCallersCommit(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	tx := openTx(t, pool)

	u := invitedWithin(t, store.Within(tx), "maria@example.com", "Maria Perez")
	if _, err := store.UserByEmail(t.Context(), "maria@example.com"); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Fatalf("UserByEmail() before commit error = %v, want ErrUserNotFound", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("Commit() error = %v, want nil", err)
	}

	held, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() after commit error = %v, want nil", err)
	}
	if held.ID != u.ID {
		t.Fatalf("UserByEmail() ID = %s, want %s", held.ID, u.ID)
	}
}

func TestWithinMintsAnInviteAsASavepoint(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	tx := openTx(t, pool)
	invites := authkit.NewInvites(authkit.InvitesConfig{Store: store.Within(tx), InviteTTL: time.Hour})

	tok, err := invites.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("Commit() error = %v, want nil", err)
	}

	activated, err := store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "hash")
	if err != nil {
		t.Fatalf("ActivateByToken() error = %v, want nil", err)
	}
	if activated != tok.UserID {
		t.Fatalf("ActivateByToken() = %s, want %s", activated, tok.UserID)
	}
}

func TestWithinRollsTheInviteBackWithTheCaller(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	tx := openTx(t, pool)
	invites := authkit.NewInvites(authkit.InvitesConfig{Store: store.Within(tx), InviteTTL: time.Hour})

	tok, err := invites.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatalf("Rollback() error = %v, want nil", err)
	}

	_, err = store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "hash")
	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Fatalf("ActivateByToken() after rollback error = %v, want ErrTokenNotFound", err)
	}
}

func TestWithinKeepsTheCallersTransactionUsableAfterARefusal(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	tx := openTx(t, pool)
	scoped := store.Within(tx)

	first := invitedWithin(t, scoped, "maria@example.com", "Maria Perez")
	issuedToken(t, scoped, first.ID, gouncer.PurposeInvite)
	second, err := gouncer.NewToken(first.ID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}
	if err := scoped.CreateToken(t.Context(), second, 1); !errors.Is(err, gouncer.ErrTokenExists) {
		t.Fatalf("CreateToken() at the cap error = %v, want ErrTokenExists", err)
	}

	later := invitedWithin(t, scoped, "lucia@example.com", "Lucia Gomez")
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("Commit() after a refused savepoint error = %v, want nil", err)
	}

	for _, want := range []gouncer.User{first, later} {
		held, err := store.UserByEmail(t.Context(), want.Email)
		if err != nil {
			t.Fatalf("UserByEmail(%q) error = %v, want nil", want.Email, err)
		}
		if held.ID != want.ID {
			t.Fatalf("UserByEmail(%q) ID = %s, want %s", want.Email, held.ID, want.ID)
		}
	}
}
