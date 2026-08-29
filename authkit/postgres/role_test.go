// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/postgres"
)

// addWithRole stores a user under the given email holding the given role.
func addWithRole(t *testing.T, store *postgres.UserStore, email, role string) gouncer.User {
	t.Helper()
	u, err := gouncer.NewUser(email, "Maria Perez", "correct horse battery")
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	u.Role = role
	if err := store.CreateUser(t.Context(), u); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return u
}

func TestUserStoreCarriesTheRoleThroughEveryRead(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addWithRole(t, store, "ada@example.com", "admin")

	byEmail, err := store.UserByEmail(t.Context(), ada.Email)
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if byEmail.Role != "admin" {
		t.Errorf("UserByEmail role = %q, want %q", byEmail.Role, "admin")
	}

	byID, err := store.UserByID(t.Context(), ada.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if byID.Role != "admin" {
		t.Errorf("UserByID role = %q, want %q", byID.Role, "admin")
	}

	listed, err := store.ListUsers(t.Context())
	if err != nil {
		t.Fatalf("ListUsers() error = %v, want nil", err)
	}
	if len(listed) != 1 || listed[0].Role != "admin" {
		t.Errorf("ListUsers roles = %v, want one admin", listed)
	}
}

func TestSetUserRoleWritesTheRole(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addWithRole(t, store, "ada@example.com", "admin")
	addWithRole(t, store, "grace@example.com", "admin")

	if err := store.SetUserRole(t.Context(), ada.ID, "editor", gouncer.Roles{"admin"}); err != nil {
		t.Fatalf("SetUserRole() error = %v, want nil", err)
	}

	held, err := store.UserByID(t.Context(), ada.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.Role != "editor" {
		t.Errorf("role = %q, want %q", held.Role, "editor")
	}
}

func TestSetUserRoleRefusesToRemoveTheLastPrivilegedAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addWithRole(t, store, "ada@example.com", "admin")
	addWithRole(t, store, "grace@example.com", "editor")

	err := store.SetUserRole(t.Context(), ada.ID, "editor", gouncer.Roles{"admin"})

	if !errors.Is(err, gouncer.ErrLastPrivileged) {
		t.Fatalf("SetUserRole() error = %v, want ErrLastPrivileged", err)
	}
	held, readErr := store.UserByID(t.Context(), ada.ID)
	if readErr != nil {
		t.Fatalf("UserByID() error = %v, want nil", readErr)
	}
	if held.Role != "admin" {
		t.Errorf("role = %q, want the refused write to leave %q", held.Role, "admin")
	}
}

func TestSetUserRoleCountsOnlyEnabledAccountsAsCover(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addWithRole(t, store, "ada@example.com", "admin")
	grace := addWithRole(t, store, "grace@example.com", "admin")
	if err := store.SetUserDisabled(t.Context(), grace.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}

	err := store.SetUserRole(t.Context(), ada.ID, "editor", gouncer.Roles{"admin"})

	if !errors.Is(err, gouncer.ErrLastPrivileged) {
		t.Errorf("SetUserRole() error = %v, want ErrLastPrivileged when the only other admin is disabled", err)
	}
}

func TestSetUserDisabledRefusesToDisableTheLastPrivilegedAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addWithRole(t, store, "ada@example.com", "admin")
	addWithRole(t, store, "grace@example.com", "editor")

	err := store.SetUserDisabledUnderCover(t.Context(), ada.ID, true, gouncer.Roles{"admin"})

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

func TestSetUserRoleLeavesOnePrivilegedAccountUnderConcurrentDemotions(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	const contenders = 8
	admins := make([]gouncer.User, contenders)
	for i := range admins {
		admins[i] = addWithRole(t, store, fmt.Sprintf("maria%d@example.com", i), "admin")
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(contenders)
	for _, held := range admins {
		go func() {
			defer wait.Done()
			<-start
			_ = store.SetUserRole(t.Context(), held.ID, "editor", gouncer.Roles{"admin"})
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
		if u.Role == "admin" {
			left++
		}
	}
	if left != 1 {
		t.Errorf("admins left = %d, want exactly 1 to survive %d concurrent demotions", left, contenders)
	}
}

func TestEnsureAdminStampsARolelessAccountInPostgres(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addWithRole(t, store, "ada@example.com", "")

	created, err := authkit.EnsureAdmin(t.Context(), store, "ada@example.com", "Ada", "password1234", "admin")

	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}
	if created {
		t.Error("EnsureAdmin() created = true, want the existing account kept")
	}
	held, err := store.UserByID(t.Context(), ada.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want the stamped account", err)
	}
	if held.Role != "admin" {
		t.Errorf("Role = %q, want the empty role stamped with admin", held.Role)
	}
}

func TestEnsureAdminLeavesAHeldRoleStandingInPostgres(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	grace := addWithRole(t, store, "grace@example.com", "editor")

	_, err := authkit.EnsureAdmin(t.Context(), store, "grace@example.com", "Grace", "password1234", "admin")

	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}
	held, err := store.UserByID(t.Context(), grace.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want the existing account", err)
	}
	if held.Role != "editor" {
		t.Errorf("Role = %q, want the held role left standing", held.Role)
	}
}

func TestEnsureAdminStampsTheLoneAccountWithoutRefusingItsOwnCover(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	only := addWithRole(t, store, "only@example.com", "")

	_, err := authkit.EnsureAdmin(t.Context(), store, "only@example.com", "Only", "password1234", "admin")

	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want the lone account stamped", err)
	}
	held, err := store.UserByID(t.Context(), only.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want the stamped account", err)
	}
	if held.Role != "admin" {
		t.Errorf("Role = %q, want the only account carried across", held.Role)
	}
}

func TestSetUserRoleWithNoPrivilegedListStampsARolelessAccount(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addWithRole(t, store, "ada@example.com", "")

	if err := store.SetUserRole(t.Context(), ada.ID, "admin", nil); err != nil {
		t.Fatalf("SetUserRole() error = %v, want nil", err)
	}

	held, err := store.UserByID(t.Context(), ada.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want the stamped account", err)
	}
	if held.Role != "admin" {
		t.Errorf("Role = %q, want the empty role stamped with admin", held.Role)
	}
}

func TestGrantRoleToRolelessAccountsFillsOnlyTheEmptyOnes(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addWithRole(t, store, "ada@example.com", "")
	grace := addWithRole(t, store, "grace@example.com", "editor")

	granted, err := store.GrantRoleToRoleless(t.Context(), "admin")
	if err != nil {
		t.Fatalf("GrantRoleToRoleless() error = %v, want nil", err)
	}
	if granted != 1 {
		t.Errorf("granted = %d, want 1", granted)
	}

	held, _ := store.UserByID(t.Context(), ada.ID)
	if held.Role != "admin" {
		t.Errorf("account holding no role = %q, want %q", held.Role, "admin")
	}
	kept, _ := store.UserByID(t.Context(), grace.ID)
	if kept.Role != "editor" {
		t.Errorf("account with a role = %q, want it left at %q", kept.Role, "editor")
	}
}

func TestGrantRoleToRolelessAccountsIsIdempotent(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	addWithRole(t, store, "ada@example.com", "")

	if _, err := store.GrantRoleToRoleless(t.Context(), "admin"); err != nil {
		t.Fatalf("first GrantRoleToRoleless() error = %v, want nil", err)
	}
	granted, err := store.GrantRoleToRoleless(t.Context(), "admin")
	if err != nil {
		t.Fatalf("second GrantRoleToRoleless() error = %v, want nil", err)
	}

	if granted != 0 {
		t.Errorf("second run granted = %d, want 0", granted)
	}
}

func TestGrantRoleToRolelessRefusesTheEmptyRole(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	ada := addWithRole(t, store, "ada@example.com", "")

	granted, err := store.GrantRoleToRoleless(t.Context(), "")

	if !errors.Is(err, gouncer.ErrEmptyRole) {
		t.Fatalf("GrantRoleToRoleless() error = %v, want ErrEmptyRole", err)
	}
	if granted != 0 {
		t.Errorf("granted = %d, want 0", granted)
	}
	held, readErr := store.UserByID(t.Context(), ada.ID)
	if readErr != nil {
		t.Fatalf("UserByID() error = %v, want nil", readErr)
	}
	if held.Role != "" {
		t.Errorf("role = %q, want the refused grant to leave it holding none", held.Role)
	}
}

func TestDisablingAnAccountRevokesItsTokens(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	tok := issuedToken(t, store, u.ID, gouncer.PurposeInvite)
	if err := store.SetUserDisabled(t.Context(), u.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	if err := store.SetUserDisabled(t.Context(), u.ID, false); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}

	_, err := store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "attacker-hash")

	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("ActivateByToken() error = %v, want the disable to have revoked the token", err)
	}
}

func TestTheGuardedDisableRevokesItsTokens(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserStore(newTestPool(t))
	u := invitedAccount(t, store)
	tok := issuedToken(t, store, u.ID, gouncer.PurposeInvite)
	if err := store.SetUserDisabledUnderCover(t.Context(), u.ID, true, gouncer.Roles{"admin"}); err != nil {
		t.Fatalf("SetUserDisabledUnderCover() error = %v, want nil", err)
	}
	if err := store.SetUserDisabledUnderCover(t.Context(), u.ID, false, gouncer.Roles{"admin"}); err != nil {
		t.Fatalf("SetUserDisabledUnderCover() error = %v, want nil", err)
	}

	_, err := store.ActivateByToken(t.Context(), tok.TokenHash, time.Now().UTC(), "attacker-hash")

	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("ActivateByToken() error = %v, want the guarded disable to have revoked the token", err)
	}
}
