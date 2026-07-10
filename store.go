// SPDX-License-Identifier: Apache-2.0

package gouncer

import (
	"context"
	"errors"
	"time"
)

// ErrUserNotFound reports that no user exists for the requested email.
var ErrUserNotFound = errors.New("gouncer: user not found")

// ErrEmailTaken reports that another user already owns the email.
var ErrEmailTaken = errors.New("gouncer: email already taken")

// ErrSessionNotFound reports that no usable session exists for a token:
// it is unknown, expired, or its user is disabled.
var ErrSessionNotFound = errors.New("gouncer: session not found")

// Store persists users and their login sessions, returning the package's
// Err* sentinels so callers can branch with [errors.Is].
type Store interface {
	// CreateUser stores u, or returns [ErrEmailTaken].
	CreateUser(ctx context.Context, u User) error

	// UserByEmail returns the user with the normalized email, or [ErrUserNotFound].
	UserByEmail(ctx context.Context, email string) (User, error)

	// CreateSession stores s.
	CreateSession(ctx context.Context, s Session) error

	// UserBySession returns the user owning the session, or [ErrSessionNotFound].
	UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (User, error)

	// DeleteSession removes the session. Removing an absent one is not an error.
	DeleteSession(ctx context.Context, tokenHash []byte) error
}
