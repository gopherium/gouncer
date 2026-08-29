// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/postgres"
)

// racers is how many goroutines contend for one token in the concurrency tests.
const racers = 8

// disablingAccount takes the row lock an in flight disable holds.
const disablingAccount = `UPDATE auth.users SET disabled = true WHERE id = $1`

// lockWait is how long a blocked issuance must stay blocked to prove it waits.
const lockWait = 300 * time.Millisecond

// lockRelease is how long a freed issuance may take to finish.
const lockRelease = 10 * time.Second

// raced runs body on racers goroutines released together, answering every error.
func raced(body func(int) error) []error {
	errs := make([]error, racers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = body(i)
		}()
	}
	close(start)
	wg.Wait()
	return errs
}

// settled counts the nil errors and reports every loser that answered
// anything other than want.
func settled(t *testing.T, errs []error, want error) int {
	t.Helper()
	won := 0
	for i, err := range errs {
		if err == nil {
			won++
			continue
		}
		if !errors.Is(err, want) {
			t.Errorf("racer %d answered %v, want %v", i, err, want)
		}
	}
	return won
}

func TestConcurrentRedemptionsSpendTheInviteOnce(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	tok := issuedToken(t, store, u.ID, gouncer.PurposeInvite)

	errs := raced(func(int) error {
		_, err := store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "argon2id-hash")
		return err
	})

	if won := settled(t, errs, gouncer.ErrTokenNotFound); won != 1 {
		t.Errorf("redemptions that landed = %d, want exactly 1", won)
	}
	held, err := store.UserByID(t.Context(), u.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.PasswordHash != "argon2id-hash" {
		t.Errorf("password hash = %q, want the one redemption that landed", held.PasswordHash)
	}
}

func TestConcurrentRedemptionsSpendTheResetOnce(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	u := storedUser(t, store)
	tok := issuedToken(t, store, u.ID, gouncer.PurposeReset)
	holder, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v, want nil", err)
	}
	defer func() { _ = holder.Rollback(t.Context()) }()
	if _, err := holder.Exec(t.Context(), disablingAccount, u.ID); err != nil {
		t.Fatalf("holding the account under a disable: %v", err)
	}

	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = store.ResetByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "argon2id-hash")
		}()
	}
	time.Sleep(lockWait)
	if err := holder.Rollback(t.Context()); err != nil {
		t.Fatalf("Rollback() error = %v, want nil", err)
	}
	wg.Wait()

	if won := settled(t, errs, gouncer.ErrTokenNotFound); won != 1 {
		t.Errorf("resets that landed = %d, want exactly 1 even though every one read the token live", won)
	}
}

func TestIssuanceWaitsForTheAccountLock(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	u := storedUser(t, store)
	tok, err := gouncer.NewToken(u.ID, gouncer.PurposeReset, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}
	holder, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v, want nil", err)
	}
	defer func() { _ = holder.Rollback(t.Context()) }()
	if _, err := holder.Exec(t.Context(), disablingAccount, u.ID); err != nil {
		t.Fatalf("holding the account under a disable: %v", err)
	}

	issued := make(chan error, 1)
	go func() { issued <- store.CreateToken(t.Context(), tok) }()

	select {
	case err := <-issued:
		t.Fatalf("CreateToken() answered %v while the account was held, want it to wait for the lock", err)
	case <-time.After(lockWait):
	}
	if err := holder.Rollback(t.Context()); err != nil {
		t.Fatalf("Rollback() error = %v, want nil", err)
	}
	select {
	case err := <-issued:
		if err != nil {
			t.Errorf("CreateToken() error = %v, want nil once the account was free", err)
		}
	case <-time.After(lockRelease):
		t.Error("CreateToken() never returned after the account was released")
	}
}

func TestIssuanceCannotStraddleADisable(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	tok, err := gouncer.NewToken(u.ID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("gouncer.NewToken() error = %v, want nil", err)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	var issueErr, disableErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		issueErr = store.CreateToken(t.Context(), tok)
	}()
	go func() {
		defer wg.Done()
		<-start
		disableErr = store.SetUserDisabledUnderCover(t.Context(), u.ID, true, gouncer.Roles{"admin"})
	}()
	close(start)
	wg.Wait()

	if disableErr != nil {
		t.Fatalf("SetUserDisabledUnderCover() error = %v, want nil", disableErr)
	}
	if issueErr != nil && !errors.Is(issueErr, gouncer.ErrUserNotFound) {
		t.Fatalf("CreateToken() error = %v, want nil or gouncer.ErrUserNotFound", issueErr)
	}

	if err := store.SetUserDisabledUnderCover(t.Context(), u.ID, false, gouncer.Roles{"admin"}); err != nil {
		t.Fatalf("re-enable error = %v, want nil", err)
	}
	_, err = store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "attacker-hash")
	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("ActivateByToken() error = %v, want no token to have survived the disable", err)
	}
}

func TestRedeemingAndDisablingDoNotDeadlock(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	tok := issuedToken(t, store, u.ID, gouncer.PurposeInvite)

	var wg sync.WaitGroup
	start := make(chan struct{})
	var redeemErr, disableErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, redeemErr = store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "argon2id-hash")
	}()
	go func() {
		defer wg.Done()
		<-start
		disableErr = store.SetUserDisabledUnderCover(t.Context(), u.ID, true, gouncer.Roles{"admin"})
	}()
	close(start)
	wg.Wait()

	for name, err := range map[string]error{"redeem": redeemErr, "disable": disableErr} {
		if err != nil && strings.Contains(err.Error(), "deadlock") {
			t.Errorf("%s answered %v, want the settled lock order to have avoided a deadlock", name, err)
		}
	}
	settledRedeem := redeemErr == nil ||
		errors.Is(redeemErr, gouncer.ErrUserNotFound) ||
		errors.Is(redeemErr, gouncer.ErrTokenNotFound)
	if !settledRedeem {
		t.Errorf("redeem answered %v, want nil or a gouncer refusal", redeemErr)
	}
	if disableErr != nil {
		t.Errorf("disable answered %v, want nil", disableErr)
	}
}

func TestTheSweepSparesAnInviteRenewedBesideIt(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewUserStore(pool)
	u := invitedAccount(t, store)
	expired := plantedToken(t, pool, u.ID, gouncer.PurposeInvite, time.Nanosecond)

	holder, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := holder.Exec(
		t.Context(),
		"SELECT id FROM auth.users WHERE id = $1 AND NOT disabled AND NOT confirmed FOR UPDATE", u.ID,
	); err != nil {
		t.Fatalf("holding the account: %v", err)
	}

	swept := make(chan error, 1)
	go func() {
		_, err := store.DeleteExpiredTokens(t.Context(), time.Now().UTC())
		swept <- err
	}()
	time.Sleep(400 * time.Millisecond)

	if _, err := holder.Exec(t.Context(), "DELETE FROM auth.tokens WHERE token_hash = $1", expired.TokenHash); err != nil {
		t.Fatalf("removing the expired token: %v", err)
	}
	renewed, err := gouncer.NewToken(u.ID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("NewToken() error = %v", err)
	}
	if _, err := holder.Exec(
		t.Context(),
		"INSERT INTO auth.tokens (token_hash, user_id, purpose, created_at, expires_at) VALUES ($1,$2,$3,$4,$5)",
		renewed.TokenHash, renewed.UserID, string(renewed.Purpose), renewed.CreatedAt, renewed.ExpiresAt,
	); err != nil {
		t.Fatalf("renewing the invite: %v", err)
	}
	if err := holder.Commit(t.Context()); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if err := <-swept; err != nil {
		t.Fatalf("DeleteExpiredTokens() error = %v", err)
	}

	if _, err := store.UserByID(t.Context(), u.ID); err != nil {
		t.Fatalf("UserByID() error = %v, want the renewed account spared", err)
	}
	if _, err := store.ActivateByToken(t.Context(), renewed.TokenHash, time.Now().UTC(), "hash"); err != nil {
		t.Errorf("ActivateByToken() error = %v, want the renewed invite to have survived the sweep", err)
	}
}
