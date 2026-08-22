// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/gopherium/gouncer"
)

// CreateAdmin provisions a user account under a role for command-line
// bootstrapping, reading the password as one line from stdin.
func CreateAdmin(
	ctx context.Context,
	store gouncer.Store,
	email string,
	name string,
	role string,
	stdin io.Reader,
	stdout io.Writer,
) error {
	if role == "" {
		return gouncer.ErrEmptyRole
	}
	_, _ = fmt.Fprint(stdout, "Password: ")
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		return errors.New("read password: no input")
	}

	u, err := gouncer.NewUser(email, name, scanner.Text())
	if err != nil {
		return err
	}
	u.Role = role
	if err := store.CreateUser(ctx, u); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "created user %s\n", u.Email)
	return nil
}
