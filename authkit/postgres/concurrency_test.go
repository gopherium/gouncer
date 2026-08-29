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

// settled counts the nil errors and reports the first that is not nil.
func settled(errs []error) (int, error) {
	won := 0
	var refused error
	for _, err := range errs {
		if err == nil {
			won++
			continue
		}
		if refused == nil {
			refused = err
		}
	}
	return won, refused
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

	won, refused := settled(errs)
	if won != 1 {
		t.Errorf("redemptions that landed = %d, want exactly 1", won)
	}
	if !errors.Is(refused, gouncer.ErrTokenNotFound) {
		t.Errorf("a losing redemption answered %v, want gouncer.ErrTokenNotFound", refused)
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

	won, refused := settled(errs)
	if won != 1 {
		t.Errorf("resets that landed = %d, want exactly 1 even though every one read the token live", won)
	}
	if !errors.Is(refused, gouncer.ErrTokenNotFound) {
		t.Errorf("a losing reset answered %v, want gouncer.ErrTokenNotFound", refused)
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
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_ = store.CreateToken(t.Context(), tok)
	}()
	go func() {
		defer wg.Done()
		<-start
		_ = store.SetUserDisabledUnderCover(t.Context(), u.ID, true, gouncer.Roles{"admin"})
	}()
	close(start)
	wg.Wait()

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
