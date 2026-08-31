// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/postgres"
)

// invitedAccount stores an unconfirmed account and returns it.
func invitedAccount(t *testing.T, store *postgres.UserStore) gouncer.User {
	t.Helper()
	u, err := gouncer.NewInvitedUser("maria@example.com", "Maria Perez")
	if err != nil {
		t.Fatalf("gouncer.NewInvitedUser() error = %v, want nil", err)
	}
	if err := store.CreateUser(t.Context(), u); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return u
}

// storedUser stores a confirmed account and returns it.
func storedUser(t *testing.T, store *postgres.UserStore) gouncer.User {
	t.Helper()
	u, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	if err := store.CreateUser(t.Context(), u); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return u
}

// issuedToken mints a token for the account and stores it.
func issuedToken(t *testing.T, store *postgres.UserStore, id uuid.UUID, purpose gouncer.TokenPurpose) gouncer.Token {
	t.Helper()
	tok, err := gouncer.NewToken(id, purpose, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}
	if err := store.CreateToken(t.Context(), tok, 1); err != nil {
		t.Fatalf("CreateToken() error = %v, want nil", err)
	}
	return tok
}

// plantedToken stores a token straight into the table, past the store's own guards.
func plantedToken(
	t *testing.T,
	pool *pgxpool.Pool,
	id uuid.UUID,
	purpose gouncer.TokenPurpose,
	ttl time.Duration,
) gouncer.Token {
	t.Helper()
	tok, err := gouncer.NewToken(id, purpose, ttl)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}
	_, err = pool.Exec(
		t.Context(),
		"INSERT INTO auth.tokens (token_hash, user_id, purpose, created_at, expires_at) VALUES ($1, $2, $3, $4, $5)",
		tok.TokenHash, tok.UserID, string(tok.Purpose), tok.CreatedAt, tok.ExpiresAt,
	)
	if err != nil {
		t.Fatalf("planting the token: %v", err)
	}
	return tok
}

func TestCreateTokenHoldsWhileALiveTokenStands(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	issuedToken(t, store, u.ID, gouncer.PurposeInvite)

	second, err := gouncer.NewToken(u.ID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}

	if err := store.CreateToken(t.Context(), second, 1); !errors.Is(err, gouncer.ErrTokenExists) {
		t.Errorf("CreateToken() error = %v, want gouncer.ErrTokenExists", err)
	}
}

func TestCreateTokenRefusesADisabledAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	if err := store.SetUserDisabled(t.Context(), u.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	tok, err := gouncer.NewToken(u.ID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}

	if err := store.CreateToken(t.Context(), tok, 1); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("CreateToken() error = %v, want gouncer.ErrUserNotFound", err)
	}
}

func TestReplaceTokenSupersedesTheStandingOne(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	first := issuedToken(t, store, u.ID, gouncer.PurposeInvite)
	second, err := gouncer.NewToken(u.ID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}

	if err := store.ReplaceToken(t.Context(), second); err != nil {
		t.Fatalf("ReplaceToken() error = %v, want nil", err)
	}

	now := time.Now().UTC()
	_, err = store.ActivateByToken(t.Context(), first.TokenHash, now, "hash")
	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("ActivateByToken(superseded) error = %v, want gouncer.ErrTokenNotFound", err)
	}
	if _, err := store.ActivateByToken(t.Context(), second.TokenHash, now, "hash"); err != nil {
		t.Errorf("ActivateByToken(replacement) error = %v, want nil", err)
	}
}

func TestReplaceTokenRefusesADisabledAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	if err := store.SetUserDisabled(t.Context(), u.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	fresh, err := gouncer.NewToken(u.ID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}

	if err := store.ReplaceToken(t.Context(), fresh); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("ReplaceToken() error = %v, want gouncer.ErrUserNotFound", err)
	}
}

func TestReplaceTokenRefusesAConfirmedAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	first := issuedToken(t, store, u.ID, gouncer.PurposeInvite)
	if _, err := store.ActivateByToken(t.Context(), first.TokenHash, time.Now().UTC(), "settled-hash"); err != nil {
		t.Fatalf("ActivateByToken() error = %v, want nil", err)
	}
	fresh, err := gouncer.NewToken(u.ID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}

	if err := store.ReplaceToken(t.Context(), fresh); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("ReplaceToken() error = %v, want gouncer.ErrUserNotFound for an activated account", err)
	}
}

func TestActivateByTokenConfirmsTheAccountAndSpendsTheToken(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	tok := issuedToken(t, store, u.ID, gouncer.PurposeInvite)

	id, err := store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "argon2id-hash")
	if err != nil {
		t.Fatalf("ActivateByToken() error = %v, want nil", err)
	}

	if id != u.ID {
		t.Errorf("id = %v, want the invited account %v", id, u.ID)
	}
	held, err := store.UserByID(t.Context(), u.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if !held.Confirmed {
		t.Error("confirmed = false, want the redemption to confirm the address")
	}
	if held.PasswordHash != "argon2id-hash" {
		t.Errorf("password hash = %q, want the redeemed one", held.PasswordHash)
	}
	_, err = store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "another")
	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("second ActivateByToken() error = %v, want gouncer.ErrTokenNotFound", err)
	}
}

func TestActivateByTokenRefusesAConfirmedAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	first := issuedToken(t, store, u.ID, gouncer.PurposeInvite)
	if _, err := store.ActivateByToken(t.Context(), first.TokenHash, time.Now().UTC(), "settled-hash"); err != nil {
		t.Fatalf("ActivateByToken() error = %v, want nil", err)
	}
	stale := issuedToken(t, store, u.ID, gouncer.PurposeInvite)

	_, err := store.ActivateByToken(t.Context(), stale.TokenHash, time.Now().UTC(), "attacker-hash")

	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("ActivateByToken() error = %v, want gouncer.ErrUserNotFound", err)
	}
	held, err := store.UserByID(t.Context(), u.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.PasswordHash != "settled-hash" {
		t.Errorf("password hash = %q, want the settled one untouched", held.PasswordHash)
	}
}

func TestActivateByTokenRefusesAnExpiredToken(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	tok := issuedToken(t, store, u.ID, gouncer.PurposeInvite)

	_, err := store.ActivateByToken(t.Context(), tok.TokenHash, tok.ExpiresAt.Add(time.Second), "hash")

	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("ActivateByToken(expired) error = %v, want gouncer.ErrTokenNotFound", err)
	}
}

func TestActivateByTokenRefusesAResetToken(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := storedUser(t, store)
	tok := issuedToken(t, store, u.ID, gouncer.PurposeReset)

	_, err := store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "hash")

	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("ActivateByToken(reset token) error = %v, want gouncer.ErrTokenNotFound", err)
	}
}

func TestResetByTokenReplacesThePasswordAndEndsEverySession(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := storedUser(t, store)
	session, err := gouncer.NewSession(u.ID)
	if err != nil {
		t.Fatalf("gouncer.NewSession() error = %v, want nil", err)
	}
	if err := store.CreateSession(t.Context(), session); err != nil {
		t.Fatalf("CreateSession() error = %v, want nil", err)
	}
	tok := issuedToken(t, store, u.ID, gouncer.PurposeReset)

	id, err := store.ResetByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "fresh-hash")
	if err != nil {
		t.Fatalf("ResetByToken() error = %v, want nil", err)
	}

	if id != u.ID {
		t.Errorf("id = %v, want %v", id, u.ID)
	}
	held, err := store.UserByID(t.Context(), u.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.PasswordHash != "fresh-hash" {
		t.Errorf("password hash = %q, want the reset one", held.PasswordHash)
	}
	_, err = store.UserBySession(t.Context(), session.TokenHash, time.Now().UTC())
	if !errors.Is(err, gouncer.ErrSessionNotFound) {
		t.Errorf("UserBySession() error = %v, want the reset to have ended it", err)
	}
}

func TestResetByTokenRefusesADisabledAccount(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	u := storedUser(t, store)
	if err := store.SetUserDisabled(t.Context(), u.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	tok := plantedToken(t, pool, u.ID, gouncer.PurposeReset, time.Hour)

	_, err := store.ResetByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "attacker-hash")

	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("ResetByToken() error = %v, want gouncer.ErrUserNotFound", err)
	}
}

func TestDeleteExpiredTokensSparesAnAccountHoldingALiveInvite(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	u := invitedAccount(t, store)
	plantedToken(t, pool, u.ID, gouncer.PurposeInvite, time.Nanosecond)
	live := plantedToken(t, pool, u.ID, gouncer.PurposeInvite, time.Hour)

	if _, err := store.DeleteExpiredTokens(t.Context(), time.Now().UTC()); err != nil {
		t.Fatalf("DeleteExpiredTokens() error = %v, want nil", err)
	}

	if _, err := store.UserByID(t.Context(), u.ID); err != nil {
		t.Fatalf("UserByID() error = %v, want the account holding a live invite spared", err)
	}
	if _, err := store.ActivateByToken(t.Context(), live.TokenHash, time.Now().UTC(), "argon2id-hash"); err != nil {
		t.Errorf("ActivateByToken() error = %v, want the live invite to have survived the sweep", err)
	}
}

func TestDeleteExpiredTokensTakesTheAccountsAnExpiredInviteStrands(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	invited := invitedAccount(t, store)
	expired, err := gouncer.NewToken(invited.ID, gouncer.PurposeInvite, time.Nanosecond)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}
	if err := store.CreateToken(t.Context(), expired, 1); err != nil {
		t.Fatalf("CreateToken() error = %v, want nil", err)
	}
	settled := storedUser(t, store)
	stale, err := gouncer.NewToken(settled.ID, gouncer.PurposeReset, time.Nanosecond)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}
	if err := store.CreateToken(t.Context(), stale, 1); err != nil {
		t.Fatalf("CreateToken() error = %v, want nil", err)
	}

	swept, err := store.DeleteExpiredTokens(t.Context(), time.Now().UTC().Add(time.Minute))
	if err != nil {
		t.Fatalf("DeleteExpiredTokens() error = %v, want nil", err)
	}

	if swept != 2 {
		t.Errorf("swept = %d, want both expired tokens", swept)
	}
	if _, err := store.UserByID(t.Context(), invited.ID); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("UserByID(invited) error = %v, want the stranded account swept", err)
	}
	if _, err := store.UserByID(t.Context(), settled.ID); err != nil {
		t.Errorf("UserByID(settled) error = %v, want the activated account spared", err)
	}
}

func TestCreateTokenStacksUpToTheCap(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := storedUser(t, store)

	for standing := 1; standing <= 3; standing++ {
		tok, err := gouncer.NewToken(u.ID, gouncer.PurposeReset, time.Hour)
		if err != nil {
			t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
		}
		if err := store.CreateToken(t.Context(), tok, 3); err != nil {
			t.Fatalf("CreateToken() number %d error = %v, want the cap to admit it", standing, err)
		}
	}

	over, err := gouncer.NewToken(u.ID, gouncer.PurposeReset, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}
	if err := store.CreateToken(t.Context(), over, 3); !errors.Is(err, gouncer.ErrTokenExists) {
		t.Errorf("CreateToken() beyond the cap error = %v, want gouncer.ErrTokenExists", err)
	}
}

func TestResetByTokenEndsEveryResetSibling(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := storedUser(t, store)
	family := make([]gouncer.Token, 3)
	for held := range family {
		tok, err := gouncer.NewToken(u.ID, gouncer.PurposeReset, time.Hour)
		if err != nil {
			t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
		}
		if err := store.CreateToken(t.Context(), tok, 3); err != nil {
			t.Fatalf("CreateToken() error = %v, want nil", err)
		}
		family[held] = tok
	}

	if _, err := store.ResetByToken(t.Context(), family[1].TokenHash, time.Now().UTC(), "fresh-hash"); err != nil {
		t.Fatalf("ResetByToken(middle link) error = %v, want nil", err)
	}

	for _, sibling := range []gouncer.Token{family[0], family[2]} {
		_, err := store.ResetByToken(t.Context(), sibling.TokenHash, time.Now().UTC(), "another-hash")
		if !errors.Is(err, gouncer.ErrTokenNotFound) {
			t.Errorf("ResetByToken(sibling) error = %v, want the family ended with the spent link", err)
		}
	}
}
