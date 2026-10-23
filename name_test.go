// SPDX-License-Identifier: Apache-2.0

package gouncer_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gopherium/gouncer"
)

func TestAccountConstructorsCleanNames(t *testing.T) {
	t.Parallel()

	constructors := map[string]func(string) (gouncer.User, error){
		"NewUser": func(name string) (gouncer.User, error) {
			return gouncer.NewUser("ada@example.com", name, "correct horse battery")
		},
		"NewInvitedUser": func(name string) (gouncer.User, error) {
			return gouncer.NewInvitedUser("ada@example.com", name)
		},
	}
	for constructor, create := range constructors {
		t.Run(constructor, func(t *testing.T) {
			for _, tc := range []struct {
				name string
				want string
			}{
				{"Ada\x00 Lovelace", "Ada Lovelace"},
				{" \tAda  \r\n\t Lovelace\n ", "Ada Lovelace"},
				{"Ad\x01a\x1f Lovelace", "Ada Lovelace"},
				{"  آدا لاولیس  ", "آدا لاولیس"},
				{"Ada\u007f\u0085\u009b Lovelace", "Ada\u007f\u0085\u009b Lovelace"},
				{"Ada\u202e\u200b Lovelace", "Ada\u202e\u200b Lovelace"},
				{"\u0085Ada", "\u0085Ada"},
				{"Ada\u0085", "Ada\u0085"},
				{"\u0085", "\u0085"},
				{" \u00a0\u0085Ada\u0085\u00a0 ", "\u0085Ada\u0085"},
				{" \u00a0Ada\u00a0 ", "Ada"},
				{"\u007fAda\u007f", "\u007fAda\u007f"},
				{"\u0080\u009bAda\u009f", "\u0080\u009bAda\u009f"},
				{"\u202eAda\u200b", "\u202eAda\u200b"},
				{strings.Repeat("a", 256) + "\x00", strings.Repeat("a", 256)},
			} {
				u, err := create(tc.name)
				if err != nil {
					t.Errorf("name %q: error = %v, want nil", tc.name, err)
					continue
				}
				if u.Name != tc.want {
					t.Errorf("name = %q, want %q", u.Name, tc.want)
				}
			}
		})
	}
}

func TestNewInvitedUserCleansEveryC0Control(t *testing.T) {
	t.Parallel()

	spaces := map[rune]string{'\t': " ", '\n': " ", '\r': " "}
	for control := rune(0); control < 0x20; control++ {
		t.Run(fmt.Sprintf("U+%04X", control), func(t *testing.T) {
			t.Parallel()

			want := "Ada" + spaces[control] + "Lovelace"
			u, err := gouncer.NewInvitedUser("ada@example.com", "Ada"+string(control)+"Lovelace")
			if err != nil {
				t.Fatalf("NewInvitedUser() error = %v, want nil", err)
			}
			if u.Name != want {
				t.Errorf("name = %q, want %q", u.Name, want)
			}
		})
	}
}

func TestNewInvitedUserRejectsNamesThatCleanToEmpty(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"\x00\x01\x1f", "Ada\xff", "\xc0\x80", "\xed\xa0\x80", "\xf4\x90\x80\x80"} {
		_, err := gouncer.NewInvitedUser("ada@example.com", name)
		if !errors.Is(err, gouncer.ErrEmptyName) {
			t.Errorf("name %q: error = %v, want %v", name, err, gouncer.ErrEmptyName)
		}
	}
}

func TestNewInvitedUserRejectsNamesStillTooLongAfterCleaning(t *testing.T) {
	t.Parallel()

	_, err := gouncer.NewInvitedUser("ada@example.com", strings.Repeat("a", 257)+"\x00")
	if !errors.Is(err, gouncer.ErrNameTooLong) {
		t.Errorf("NewInvitedUser() error = %v, want %v", err, gouncer.ErrNameTooLong)
	}
}
