// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// roleWriter is the store surface a role stamp writes through.
type roleWriter interface {
	SetUserRole(ctx context.Context, id uuid.UUID, role string, privileged gouncer.Roles) error
}

// EnsureAdmin creates a user account under a role unless the email is
// already taken, stamping the role when the taken account holds none.
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
		return false, stampRoleless(ctx, store, u.Email, role)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// stampRoleless gives the role to the named account when it holds none.
func stampRoleless(ctx context.Context, store gouncer.Store, email, role string) error {
	writer, ok := store.(roleWriter)
	if !ok {
		return nil
	}
	held, err := store.UserByEmail(ctx, email)
	if err != nil {
		return err
	}
	if held.Role != "" {
		return nil
	}
	return writer.SetUserRole(ctx, held.ID, role, nil)
}
