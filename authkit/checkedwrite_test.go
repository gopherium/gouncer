// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

// checkedWrites is an administration with an actor and Maria Perez stored.
type checkedWrites struct {
	admin *authkit.AdminHandlers
	store *testkit.Store
	actor gouncer.User
	maria gouncer.User
}

// newCheckedWrites stores the actor and Maria Perez holding the roles, administered over the ladder guarding admin.
func newCheckedWrites(t *testing.T, actorRole, mariaRole string) checkedWrites {
	t.Helper()
	store := testkit.NewStore()
	actor := addAdaAs(t, store, actorRole)
	maria := addMariaAs(t, store, mariaRole)
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: store, Privileged: gouncer.Roles{"admin"}, Roles: ladder()})
	return checkedWrites{admin: admin, store: store, actor: actor, maria: maria}
}

// wantRole fails unless the stored account holds the role.
func (c checkedWrites) wantRole(t *testing.T, account gouncer.User, role string) {
	t.Helper()
	if held := c.store.Users[account.ID].Role; held != role {
		t.Errorf("role = %q, want %q", held, role)
	}
}

// wantDisabled fails unless the stored account is disabled as given.
func (c checkedWrites) wantDisabled(t *testing.T, account gouncer.User, disabled bool) {
	t.Helper()
	if held := c.store.Users[account.ID].Disabled; held != disabled {
		t.Errorf("disabled = %v, want %v", held, disabled)
	}
}

func TestSetAccountRoleFromWritesTheRoleOfAnAccountStillHoldingHeld(t *testing.T) {
	t.Parallel()

	c := newCheckedWrites(t, "admin", "member")

	if err := c.admin.SetAccountRoleFrom(t.Context(), c.actor.ID, c.maria.ID, "member", "moderator"); err != nil {
		t.Fatalf("SetAccountRoleFrom(member to moderator) error = %v, want nil", err)
	}
	c.wantRole(t, c.maria, "moderator")
}

func TestSetAccountRoleFromRefusesAnAccountWhoseRoleMoved(t *testing.T) {
	t.Parallel()

	c := newCheckedWrites(t, "admin", "moderator")

	err := c.admin.SetAccountRoleFrom(t.Context(), c.actor.ID, c.maria.ID, "member", "organizer")

	if !errors.Is(err, authkit.ErrRoleChanged) {
		t.Errorf("SetAccountRoleFrom(held member) error = %v, want ErrRoleChanged", err)
	}
	c.wantRole(t, c.maria, "moderator")
}

func TestSetAccountRoleFromRefusesTheActorsOwnRole(t *testing.T) {
	t.Parallel()

	c := newCheckedWrites(t, "admin", "member")

	err := c.admin.SetAccountRoleFrom(t.Context(), c.actor.ID, c.actor.ID, "admin", "member")

	if !errors.Is(err, authkit.ErrSelfRole) {
		t.Errorf("SetAccountRoleFrom(own) error = %v, want ErrSelfRole", err)
	}
	c.wantRole(t, c.actor, "admin")
}

func TestSetAccountRoleFromRefusesARoleTheSourceDoesNotKnow(t *testing.T) {
	t.Parallel()

	for _, role := range []string{"ghost", ""} {
		t.Run("role "+strconv.Quote(role), func(t *testing.T) {
			t.Parallel()

			c := newCheckedWrites(t, "admin", "member")

			err := c.admin.SetAccountRoleFrom(t.Context(), c.actor.ID, c.maria.ID, "member", role)

			if !errors.Is(err, authkit.ErrUnknownRole) {
				t.Errorf("SetAccountRoleFrom(%q) error = %v, want ErrUnknownRole", role, err)
			}
			c.wantRole(t, c.maria, "member")
		})
	}
}

func TestSetAccountRoleFromRefusesRemovingTheLastPrivilegedAccount(t *testing.T) {
	t.Parallel()

	c := newCheckedWrites(t, "moderator", "admin")

	err := c.admin.SetAccountRoleFrom(t.Context(), c.actor.ID, c.maria.ID, "admin", "member")

	if !errors.Is(err, gouncer.ErrLastPrivileged) {
		t.Errorf("SetAccountRoleFrom(last admin) error = %v, want ErrLastPrivileged", err)
	}
	c.wantRole(t, c.maria, "admin")
}

// disableFrom stores Maria Perez holding stored and the opposite of disabled, then writes disabled from held member.
func disableFrom(t *testing.T, stored string, disabled bool) (checkedWrites, error) {
	t.Helper()
	c := newCheckedWrites(t, "admin", stored)
	if !disabled {
		disable(c.store, c.maria)
	}
	return c, c.admin.SetAccountDisabledFrom(t.Context(), c.actor.ID, c.maria.ID, "member", disabled)
}

// eachDisabled runs check in a parallel subtest disabling and in another re-enabling.
func eachDisabled(t *testing.T, check func(t *testing.T, disabled bool)) {
	t.Helper()
	for _, disabled := range []bool{true, false} {
		t.Run("disabled "+strconv.FormatBool(disabled), func(t *testing.T) {
			t.Parallel()
			check(t, disabled)
		})
	}
}

func TestSetAccountDisabledFromUpdatesAnAccountStillHoldingHeld(t *testing.T) {
	t.Parallel()

	eachDisabled(t, func(t *testing.T, disabled bool) {
		c, err := disableFrom(t, "member", disabled)

		if err != nil {
			t.Fatalf("SetAccountDisabledFrom(%v) error = %v, want nil", disabled, err)
		}
		c.wantDisabled(t, c.maria, disabled)
	})
}

func TestSetAccountDisabledFromRefusesAnAccountWhoseRoleMoved(t *testing.T) {
	t.Parallel()

	eachDisabled(t, func(t *testing.T, disabled bool) {
		c, err := disableFrom(t, "moderator", disabled)

		if !errors.Is(err, authkit.ErrRoleChanged) {
			t.Errorf("SetAccountDisabledFrom(%v, held member) error = %v, want ErrRoleChanged", disabled, err)
		}
		c.wantDisabled(t, c.maria, !disabled)
		c.wantRole(t, c.maria, "moderator")
	})
}

func TestSetAccountDisabledFromRefusesTheActorDisablingItself(t *testing.T) {
	t.Parallel()

	c := newCheckedWrites(t, "admin", "member")

	err := c.admin.SetAccountDisabledFrom(t.Context(), c.actor.ID, c.actor.ID, "admin", true)

	if !errors.Is(err, authkit.ErrSelfDisable) {
		t.Errorf("SetAccountDisabledFrom(own) error = %v, want ErrSelfDisable", err)
	}
	c.wantDisabled(t, c.actor, false)
}

func TestSetAccountDisabledFromRefusesDisablingTheLastPrivilegedAccount(t *testing.T) {
	t.Parallel()

	c := newCheckedWrites(t, "moderator", "admin")

	err := c.admin.SetAccountDisabledFrom(t.Context(), c.actor.ID, c.maria.ID, "admin", true)

	if !errors.Is(err, gouncer.ErrLastPrivileged) {
		t.Errorf("SetAccountDisabledFrom(last admin) error = %v, want ErrLastPrivileged", err)
	}
	c.wantDisabled(t, c.maria, false)
}

func eachLostRole(t *testing.T, check func(t *testing.T, c checkedWrites, held string)) {
	t.Helper()
	for _, held := range []string{"withdrawn", ""} {
		t.Run("role "+strconv.Quote(held), func(t *testing.T) {
			t.Parallel()
			check(t, newCheckedWrites(t, "admin", held), held)
		})
	}
}

func TestSetAccountDisabledFromRefusesReenablingAnAccountHoldingNoDeclaredRole(t *testing.T) {
	t.Parallel()

	eachLostRole(t, func(t *testing.T, c checkedWrites, held string) {
		disable(c.store, c.maria)

		err := c.admin.SetAccountDisabledFrom(t.Context(), c.actor.ID, c.maria.ID, held, false)

		if !errors.Is(err, authkit.ErrBeyondReach) {
			t.Errorf("SetAccountDisabledFrom(%q, enable) error = %v, want ErrBeyondReach", held, err)
		}
		c.wantDisabled(t, c.maria, true)
	})
}

func TestCheckedWritesLetAnAccountHoldingNoDeclaredRoleBackOnceItHoldsOne(t *testing.T) {
	t.Parallel()

	eachLostRole(t, func(t *testing.T, c checkedWrites, held string) {
		if err := c.admin.SetAccountDisabledFrom(t.Context(), c.actor.ID, c.maria.ID, held, true); err != nil {
			t.Fatalf("SetAccountDisabledFrom(%q, disable) error = %v, want nil", held, err)
		}
		if err := c.admin.SetAccountRoleFrom(t.Context(), c.actor.ID, c.maria.ID, held, "member"); err != nil {
			t.Fatalf("SetAccountRoleFrom(%q to member) error = %v, want nil", held, err)
		}
		if err := c.admin.SetAccountDisabledFrom(t.Context(), c.actor.ID, c.maria.ID, "member", false); err != nil {
			t.Fatalf("SetAccountDisabledFrom(member, enable) error = %v, want nil", err)
		}
		c.wantRole(t, c.maria, "member")
		c.wantDisabled(t, c.maria, false)
	})
}

func TestCheckedWritesWithoutARoleSourceSkipTheHeldRole(t *testing.T) {
	t.Parallel()

	c := newCheckedWrites(t, "admin", "moderator")
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: c.store, Privileged: gouncer.Roles{"admin"}})

	if err := admin.SetAccountRoleFrom(t.Context(), c.actor.ID, c.maria.ID, "member", "editor"); err != nil {
		t.Errorf("SetAccountRoleFrom(held member) error = %v, want nil without a role source", err)
	}
	if err := admin.SetAccountDisabledFrom(t.Context(), c.actor.ID, c.maria.ID, "member", true); err != nil {
		t.Errorf("SetAccountDisabledFrom(held member) error = %v, want nil without a role source", err)
	}
	c.wantRole(t, c.maria, "editor")
	c.wantDisabled(t, c.maria, true)
}
