// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/gouncer/authkit/postgres"
)

// addRanked stores a user under the given email holding the given rank.
func addRanked(t *testing.T, store *postgres.UserStore, email, rank string) gouncer.User {
	t.Helper()
	u, err := gouncer.NewUser(email, "Maria Perez", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	u.Rank = rank
	if err := store.CreateUser(t.Context(), u); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return u
}

func TestUserStoreCarriesTheRankThroughEveryRead(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addRanked(t, store, "ada@example.com", "admin")

	byEmail, err := store.UserByEmail(t.Context(), ada.Email)
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if byEmail.Rank != "admin" {
		t.Errorf("UserByEmail rank = %q, want %q", byEmail.Rank, "admin")
	}

	byID, err := store.UserByID(t.Context(), ada.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if byID.Rank != "admin" {
		t.Errorf("UserByID rank = %q, want %q", byID.Rank, "admin")
	}

	listed, err := store.ListUsers(t.Context())
	if err != nil {
		t.Fatalf("ListUsers() error = %v, want nil", err)
	}
	if len(listed) != 1 || listed[0].Rank != "admin" {
		t.Errorf("ListUsers ranks = %v, want one admin", listed)
	}
}

func TestSetUserRankWritesTheRank(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addRanked(t, store, "ada@example.com", "admin")
	addRanked(t, store, "grace@example.com", "admin")

	if err := store.SetUserRank(t.Context(), ada.ID, "editor", gouncer.Ranks{"admin"}); err != nil {
		t.Fatalf("SetUserRank() error = %v, want nil", err)
	}

	held, err := store.UserByID(t.Context(), ada.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.Rank != "editor" {
		t.Errorf("rank = %q, want %q", held.Rank, "editor")
	}
}

func TestSetUserRankRefusesToRemoveTheLastPrivilegedAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addRanked(t, store, "ada@example.com", "admin")
	addRanked(t, store, "grace@example.com", "editor")

	err := store.SetUserRank(t.Context(), ada.ID, "editor", gouncer.Ranks{"admin"})

	if !errors.Is(err, gouncer.ErrLastPrivileged) {
		t.Fatalf("SetUserRank() error = %v, want ErrLastPrivileged", err)
	}
	held, readErr := store.UserByID(t.Context(), ada.ID)
	if readErr != nil {
		t.Fatalf("UserByID() error = %v, want nil", readErr)
	}
	if held.Rank != "admin" {
		t.Errorf("rank = %q, want the refused write to leave %q", held.Rank, "admin")
	}
}

func TestSetUserRankCountsOnlyEnabledAccountsAsCover(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addRanked(t, store, "ada@example.com", "admin")
	grace := addRanked(t, store, "grace@example.com", "admin")
	if err := store.SetUserDisabled(t.Context(), grace.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}

	err := store.SetUserRank(t.Context(), ada.ID, "editor", gouncer.Ranks{"admin"})

	if !errors.Is(err, gouncer.ErrLastPrivileged) {
		t.Errorf("SetUserRank() error = %v, want ErrLastPrivileged when the only other admin is disabled", err)
	}
}

func TestSetUserDisabledRefusesToDisableTheLastPrivilegedAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addRanked(t, store, "ada@example.com", "admin")
	addRanked(t, store, "grace@example.com", "editor")

	err := store.SetUserDisabledUnderCover(t.Context(), ada.ID, true, gouncer.Ranks{"admin"})

	if !errors.Is(err, gouncer.ErrLastPrivileged) {
		t.Fatalf("SetUserDisabledUnderCover() error = %v, want ErrLastPrivileged", err)
	}
	held, readErr := store.UserByID(t.Context(), ada.ID)
	if readErr != nil {
		t.Fatalf("UserByID() error = %v, want nil", readErr)
	}
	if held.Disabled {
		t.Error("the account was disabled, want the refused write to leave it enabled")
	}
}

func TestSetUserRankLeavesOnePrivilegedAccountUnderConcurrentDemotions(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	const contenders = 8
	admins := make([]gouncer.User, contenders)
	for i := range admins {
		admins[i] = addRanked(t, store, fmt.Sprintf("maria%d@example.com", i), "admin")
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(contenders)
	for _, held := range admins {
		go func() {
			defer wait.Done()
			<-start
			_ = store.SetUserRank(t.Context(), held.ID, "editor", gouncer.Ranks{"admin"})
		}()
	}
	close(start)
	wait.Wait()

	listed, err := store.ListUsers(t.Context())
	if err != nil {
		t.Fatalf("ListUsers() error = %v, want nil", err)
	}
	left := 0
	for _, u := range listed {
		if u.Rank == "admin" {
			left++
		}
	}
	if left != 1 {
		t.Errorf("admins left = %d, want exactly 1 to survive %d concurrent demotions", left, contenders)
	}
}

func TestGrantRankToRanklessAccountsFillsOnlyTheEmptyOnes(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addRanked(t, store, "ada@example.com", "")
	grace := addRanked(t, store, "grace@example.com", "editor")

	granted, err := store.GrantRankToRankless(t.Context(), "admin")
	if err != nil {
		t.Fatalf("GrantRankToRankless() error = %v, want nil", err)
	}
	if granted != 1 {
		t.Errorf("granted = %d, want 1", granted)
	}

	held, _ := store.UserByID(t.Context(), ada.ID)
	if held.Rank != "admin" {
		t.Errorf("rankless account rank = %q, want %q", held.Rank, "admin")
	}
	kept, _ := store.UserByID(t.Context(), grace.ID)
	if kept.Rank != "editor" {
		t.Errorf("ranked account rank = %q, want it left at %q", kept.Rank, "editor")
	}
}

func TestGrantRankToRanklessAccountsIsIdempotent(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	addRanked(t, store, "ada@example.com", "")

	if _, err := store.GrantRankToRankless(t.Context(), "admin"); err != nil {
		t.Fatalf("first GrantRankToRankless() error = %v, want nil", err)
	}
	granted, err := store.GrantRankToRankless(t.Context(), "admin")
	if err != nil {
		t.Fatalf("second GrantRankToRankless() error = %v, want nil", err)
	}

	if granted != 0 {
		t.Errorf("second run granted = %d, want 0", granted)
	}
}
