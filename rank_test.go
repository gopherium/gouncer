// SPDX-License-Identifier: Apache-2.0

package gouncer_test

import (
	"testing"

	"github.com/gopherium/gouncer"
)

func TestRanksReportsWhetherARankIsAmongThem(t *testing.T) {
	t.Parallel()

	privileged := gouncer.Ranks{"admin", "owner"}

	for _, held := range []struct {
		rank string
		want bool
	}{
		{"admin", true},
		{"owner", true},
		{"editor", false},
		{"", false},
		{"Admin", false},
	} {
		if got := privileged.Holds(held.rank); got != held.want {
			t.Errorf("Ranks%v.Holds(%q) = %v, want %v", privileged, held.rank, got, held.want)
		}
	}
}

func TestRanksHoldsNothingWhenEmpty(t *testing.T) {
	t.Parallel()

	var none gouncer.Ranks

	if none.Holds("admin") {
		t.Error("an empty Ranks holds admin, want it to hold nothing")
	}
}

func TestNewUserTakesNoRank(t *testing.T) {
	t.Parallel()

	u, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", "correct horse battery")
	if err != nil {
		t.Fatalf("NewUser() error = %v, want nil", err)
	}

	if u.Rank != "" {
		t.Errorf("rank = %q, want the empty rank a store fills in", u.Rank)
	}
}
