// SPDX-License-Identifier: Apache-2.0

// Package testkit provides test doubles for authkit consumers.
package testkit

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// Store is an in-memory user and session store encoding the gouncer
// contract semantics for tests. Set an Err field to force that method
// to fail.
type Store struct {
	Users    map[uuid.UUID]gouncer.User
	Sessions map[string]gouncer.Session

	LookupErr        error
	SessionErr       error
	CreateSessionErr error
	DeleteErr        error
	ListUsersErr     error
	CreateUserErr    error
	SetDisabledErr   error
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{
		Users:    map[uuid.UUID]gouncer.User{},
		Sessions: map[string]gouncer.Session{},
	}
}

// AddUser stores and returns a user built from the given credentials.
func (s *Store) AddUser(tb testing.TB, email, name, password string) gouncer.User {
	tb.Helper()
	u, err := gouncer.NewUser(email, name, password)
	if err != nil {
		tb.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	s.Users[u.ID] = u
	return u
}

// CreateUser stores u, or returns gouncer.ErrEmailTaken for a known email.
func (s *Store) CreateUser(_ context.Context, u gouncer.User) error {
	if s.CreateUserErr != nil {
		return s.CreateUserErr
	}
	for _, existing := range s.Users {
		if existing.Email == u.Email {
			return gouncer.ErrEmailTaken
		}
	}
	s.Users[u.ID] = u
	return nil
}

// UserByEmail returns the user with the normalized email, or gouncer.ErrUserNotFound.
func (s *Store) UserByEmail(_ context.Context, email string) (gouncer.User, error) {
	if s.LookupErr != nil {
		return gouncer.User{}, s.LookupErr
	}
	for _, u := range s.Users {
		if u.Email == email {
			return u, nil
		}
	}
	return gouncer.User{}, gouncer.ErrUserNotFound
}

// CreateSession stores sess.
func (s *Store) CreateSession(_ context.Context, sess gouncer.Session) error {
	if s.CreateSessionErr != nil {
		return s.CreateSessionErr
	}
	s.Sessions[string(sess.TokenHash)] = sess
	return nil
}

// UserBySession returns the user owning a live session, or gouncer.ErrSessionNotFound.
func (s *Store) UserBySession(_ context.Context, tokenHash []byte, now time.Time) (gouncer.User, error) {
	if s.SessionErr != nil {
		return gouncer.User{}, s.SessionErr
	}
	sess, ok := s.Sessions[string(tokenHash)]
	if !ok || !sess.ExpiresAt.After(now) {
		return gouncer.User{}, gouncer.ErrSessionNotFound
	}
	u, ok := s.Users[sess.UserID]
	if !ok || u.Disabled {
		return gouncer.User{}, gouncer.ErrSessionNotFound
	}
	return u, nil
}

// DeleteSession removes the session. Removing an absent one is not an error.
func (s *Store) DeleteSession(_ context.Context, tokenHash []byte) error {
	if s.DeleteErr != nil {
		return s.DeleteErr
	}
	delete(s.Sessions, string(tokenHash))
	return nil
}

// ListUsers returns every account ordered by name then id, with password hashes stripped.
func (s *Store) ListUsers(_ context.Context) ([]gouncer.User, error) {
	if s.ListUsersErr != nil {
		return nil, s.ListUsersErr
	}
	users := slices.Collect(maps.Values(s.Users))
	slices.SortFunc(users, func(a, b gouncer.User) int {
		if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	for i := range users {
		users[i].PasswordHash = ""
	}
	return users, nil
}

// SetUserDisabled updates whether the account may log in, revoking its sessions on disable.
func (s *Store) SetUserDisabled(_ context.Context, id uuid.UUID, disabled bool) error {
	if s.SetDisabledErr != nil {
		return s.SetDisabledErr
	}
	u, ok := s.Users[id]
	if !ok {
		return gouncer.ErrUserNotFound
	}
	u.Disabled = disabled
	s.Users[id] = u
	if disabled {
		maps.DeleteFunc(s.Sessions, func(_ string, sess gouncer.Session) bool {
			return sess.UserID == id
		})
	}
	return nil
}
