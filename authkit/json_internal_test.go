// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"strings"
	"testing"

	"github.com/gopherium/gouncer"
)

// reportedLimit returns the number an error body carries under the meta key.
func reportedLimit(code, key string) (int, bool) {
	for _, held := range authErrors {
		if held.response.Code != code {
			continue
		}
		reported, ok := held.response.Meta[key].(int)
		return reported, ok
	}
	return 0, false
}

func TestErrorLimitsMatchGouncer(t *testing.T) {
	t.Parallel()

	limits := []struct {
		name    string
		code    string
		key     string
		allowed func(held int) error
		refused func(held int) error
	}{
		{
			name: "name length", code: "name_too_long", key: "max",
			allowed: func(held int) error {
				_, err := gouncer.NewUser("maria@example.com", strings.Repeat("a", held), "password1234")
				return err
			},
			refused: func(held int) error {
				_, err := gouncer.NewUser("maria@example.com", strings.Repeat("a", held+1), "password1234")
				return err
			},
		},
		{
			name: "password minimum", code: "password_too_short", key: "min",
			allowed: func(held int) error {
				_, err := gouncer.NewUser("maria@example.com", "Maria Perez", strings.Repeat("a", held))
				return err
			},
			refused: func(held int) error {
				_, err := gouncer.NewUser("maria@example.com", "Maria Perez", strings.Repeat("a", held-1))
				return err
			},
		},
		{
			name: "password maximum", code: "password_too_long", key: "max",
			allowed: func(held int) error {
				_, err := gouncer.NewUser("maria@example.com", "Maria Perez", strings.Repeat("a", held))
				return err
			},
			refused: func(held int) error {
				_, err := gouncer.NewUser("maria@example.com", "Maria Perez", strings.Repeat("a", held+1))
				return err
			},
		},
	}
	for _, limit := range limits {
		t.Run(limit.name, func(t *testing.T) {
			t.Parallel()
			reported, found := reportedLimit(limit.code, limit.key)
			if !found {
				t.Fatalf("no error body reports %q under %q", limit.code, limit.key)
			}
			if err := limit.allowed(reported); err != nil {
				t.Errorf("gouncer refused a value at the reported bound %d: %v", reported, err)
			}
			if err := limit.refused(reported); err == nil {
				t.Errorf("gouncer accepted a value beyond the reported bound %d, want it refused", reported)
			}
		})
	}
}
