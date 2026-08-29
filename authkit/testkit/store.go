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
	Tokens   map[string]gouncer.Token

	LookupErr        error
	SessionErr       error
	CreateSessionErr error
	DeleteErr        error
	ListUsersErr     error
	CreateUserErr    error
	SetDisabledErr   error
	SetRoleErr       error
	TokenErr         error
	ActivateErr      error
	ResetErr         error

	// DisabledUnderCover counts the disables that went through the guarded write.
	DisabledUnderCover int
	// CoverGiven holds the privileged roles the last guarded write was handed.
	CoverGiven gouncer.Roles
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{
		Users:    map[uuid.UUID]gouncer.User{},
		Sessions: map[string]gouncer.Session{},
		Tokens:   map[string]gouncer.Token{},
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

// UserByID returns the user with the given id, disabled or not, or
// gouncer.ErrUserNotFound.
func (s *Store) UserByID(_ context.Context, id uuid.UUID) (gouncer.User, error) {
	if s.LookupErr != nil {
		return gouncer.User{}, s.LookupErr
	}
	u, ok := s.Users[id]
	if !ok {
		return gouncer.User{}, gouncer.ErrUserNotFound
	}
	return u, nil
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
		maps.DeleteFunc(s.Tokens, func(_ string, tok gouncer.Token) bool {
			return tok.UserID == id
		})
	}
	return nil
}

// SetUserRole writes the role an account holds, or returns the configured error.
func (s *Store) SetUserRole(_ context.Context, id uuid.UUID, role string, privileged gouncer.Roles) error {
	s.CoverGiven = privileged
	if s.SetRoleErr != nil {
		return s.SetRoleErr
	}
	u, ok := s.Users[id]
	if !ok {
		return gouncer.ErrUserNotFound
	}
	u.Role = role
	s.Users[id] = u
	return nil
}

// CreateToken stores t, or returns gouncer.ErrTokenExists while a live
// token stands for the same user and purpose.
func (s *Store) CreateToken(_ context.Context, t gouncer.Token) error {
	if s.TokenErr != nil {
		return s.TokenErr
	}
	if u, ok := s.Users[t.UserID]; !ok || u.Disabled {
		return gouncer.ErrUserNotFound
	}
	for _, existing := range s.Tokens {
		if existing.UserID == t.UserID && existing.Purpose == t.Purpose && existing.ExpiresAt.After(time.Now().UTC()) {
			return gouncer.ErrTokenExists
		}
	}
	s.Tokens[string(t.TokenHash)] = t
	return nil
}

// ReplaceToken stores t in place of every token the account holds for
// the same purpose, or returns gouncer.ErrUserNotFound for an unknown
// or disabled account, replacing nothing when it refuses.
func (s *Store) ReplaceToken(_ context.Context, t gouncer.Token) error {
	if s.TokenErr != nil {
		return s.TokenErr
	}
	if u, ok := s.Users[t.UserID]; !ok || u.Disabled {
		return gouncer.ErrUserNotFound
	}
	maps.DeleteFunc(s.Tokens, func(_ string, held gouncer.Token) bool {
		return held.UserID == t.UserID && held.Purpose == t.Purpose
	})
	s.Tokens[string(t.TokenHash)] = t
	return nil
}

// ActivateByToken spends the invite token and confirms the account it
// names, or returns gouncer.ErrTokenNotFound for a spent or expired
// token and gouncer.ErrUserNotFound for an unknown, disabled or already
// confirmed account, spending nothing when it refuses.
func (s *Store) ActivateByToken(
	_ context.Context,
	tokenHash []byte,
	now time.Time,
	passwordHash string,
) (uuid.UUID, error) {
	if s.TokenErr != nil {
		return uuid.Nil, s.TokenErr
	}
	if s.ActivateErr != nil {
		return uuid.Nil, s.ActivateErr
	}
	t, ok := s.Tokens[string(tokenHash)]
	if !ok || t.Purpose != gouncer.PurposeInvite || !t.ExpiresAt.After(now) {
		return uuid.Nil, gouncer.ErrTokenNotFound
	}
	u, ok := s.Users[t.UserID]
	if !ok || u.Disabled || u.Confirmed {
		return uuid.Nil, gouncer.ErrUserNotFound
	}
	u.PasswordHash = passwordHash
	u.Confirmed = true
	s.Users[t.UserID] = u
	delete(s.Tokens, string(tokenHash))
	return t.UserID, nil
}

// ResetByToken spends the reset token, stores the password hash and ends
// every session the account it names holds, or returns
// gouncer.ErrTokenNotFound for a spent or expired token and
// gouncer.ErrUserNotFound for an unknown or disabled account, spending
// nothing when it refuses.
func (s *Store) ResetByToken(
	_ context.Context,
	tokenHash []byte,
	now time.Time,
	passwordHash string,
) (uuid.UUID, error) {
	if s.TokenErr != nil {
		return uuid.Nil, s.TokenErr
	}
	if s.ResetErr != nil {
		return uuid.Nil, s.ResetErr
	}
	t, ok := s.Tokens[string(tokenHash)]
	if !ok || t.Purpose != gouncer.PurposeReset || !t.ExpiresAt.After(now) {
		return uuid.Nil, gouncer.ErrTokenNotFound
	}
	u, ok := s.Users[t.UserID]
	if !ok || u.Disabled {
		return uuid.Nil, gouncer.ErrUserNotFound
	}
	u.PasswordHash = passwordHash
	s.Users[t.UserID] = u
	maps.DeleteFunc(s.Sessions, func(_ string, sess gouncer.Session) bool {
		return sess.UserID == t.UserID
	})
	delete(s.Tokens, string(tokenHash))
	return t.UserID, nil
}

// DeleteExpiredTokens removes expired tokens and the unconfirmed
// accounts an expired invite leaves behind, reporting how many tokens went.
func (s *Store) DeleteExpiredTokens(_ context.Context, now time.Time) (int64, error) {
	if s.TokenErr != nil {
		return 0, s.TokenErr
	}
	var count int64
	for hash, t := range s.Tokens {
		if t.ExpiresAt.After(now) {
			continue
		}
		if t.Purpose == gouncer.PurposeInvite {
			if u, ok := s.Users[t.UserID]; ok && !u.Confirmed {
				delete(s.Users, t.UserID)
			}
		}
		delete(s.Tokens, hash)
		count++
	}
	return count, nil
}

// SetUserDisabledUnderCover disables an account under the guard, counting the call.
func (s *Store) SetUserDisabledUnderCover(
	ctx context.Context,
	id uuid.UUID,
	disabled bool,
	privileged gouncer.Roles,
) error {
	s.CoverGiven = privileged
	s.DisabledUnderCover++
	return s.SetUserDisabled(ctx, id, disabled)
}
