// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"

	"github.com/gopherium/gouncer"
)

// EnsureAdmin creates a user account under a role unless the email is
// already taken, reporting whether it created the account.
func EnsureAdmin(ctx context.Context, store gouncer.Store, email, name, password, role string) (bool, error) {
	if role == "" {
		return false, gouncer.ErrEmptyRole
	}
	u, err := gouncer.NewUser(email, name, password)
	if err != nil {
		return false, err
	}
	u.Role = role
	err = store.CreateUser(ctx, u)
	if errors.Is(err, gouncer.ErrEmailTaken) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
