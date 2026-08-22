// SPDX-License-Identifier: Apache-2.0

package gouncer_test

import (
	"testing"

	"github.com/gopherium/gouncer"
)

func TestRolesReportsWhetherARoleIsAmongThem(t *testing.T) {
	t.Parallel()

	privileged := gouncer.Roles{"admin", "owner"}

	for _, held := range []struct {
		role string
		want bool
	}{
		{"admin", true},
		{"owner", true},
		{"editor", false},
		{"", false},
		{"Admin", false},
	} {
		if got := privileged.Holds(held.role); got != held.want {
			t.Errorf("Roles%v.Holds(%q) = %v, want %v", privileged, held.role, got, held.want)
		}
	}
}

func TestRolesNeverHoldsTheEmptyRole(t *testing.T) {
	t.Parallel()

	for _, configured := range []gouncer.Roles{{""}, {"admin", ""}, {"admin"}} {
		if configured.Holds("") {
			t.Errorf("Roles%v.Holds(\"\") = true, want an account holding no role never privileged", configured)
		}
	}
}

func TestRolesHoldsNothingWhenEmpty(t *testing.T) {
	t.Parallel()

	var none gouncer.Roles

	if none.Holds("admin") {
		t.Error("an empty Roles holds admin, want it to hold nothing")
	}
}

func TestNewUserTakesNoRole(t *testing.T) {
	t.Parallel()

	u, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", "correct horse battery")
	if err != nil {
		t.Fatalf("NewUser() error = %v, want nil", err)
	}

	if u.Role != "" {
		t.Errorf("role = %q, want the empty role a store fills in", u.Role)
	}
}
