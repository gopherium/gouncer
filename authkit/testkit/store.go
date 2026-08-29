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
	for _, existing := range s.Tokens {
		if existing.UserID == t.UserID && existing.Purpose == t.Purpose && existing.ExpiresAt.After(time.Now().UTC()) {
			return gouncer.ErrTokenExists
		}
	}
	s.Tokens[string(t.TokenHash)] = t
	return nil
}

// ConsumeToken removes and returns the live token behind the hash and
// purpose, or gouncer.ErrTokenNotFound.
func (s *Store) ConsumeToken(
	_ context.Context,
	tokenHash []byte,
	purpose gouncer.TokenPurpose,
	now time.Time,
) (gouncer.Token, error) {
	if s.TokenErr != nil {
		return gouncer.Token{}, s.TokenErr
	}
	t, ok := s.Tokens[string(tokenHash)]
	if !ok || t.Purpose != purpose || !t.ExpiresAt.After(now) {
		return gouncer.Token{}, gouncer.ErrTokenNotFound
	}
	delete(s.Tokens, string(tokenHash))
	return t, nil
}

// DeleteTokensForUser removes every token the user holds for the purpose.
func (s *Store) DeleteTokensForUser(_ context.Context, id uuid.UUID, purpose gouncer.TokenPurpose) error {
	if s.TokenErr != nil {
		return s.TokenErr
	}
	maps.DeleteFunc(s.Tokens, func(_ string, t gouncer.Token) bool {
		return t.UserID == id && t.Purpose == purpose
	})
	return nil
}

// ActivateAccount stores the password hash and confirms the account, or
// returns gouncer.ErrUserNotFound for an unknown, disabled or already
// confirmed one.
func (s *Store) ActivateAccount(_ context.Context, id uuid.UUID, passwordHash string) error {
	if s.ActivateErr != nil {
		return s.ActivateErr
	}
	u, ok := s.Users[id]
	if !ok || u.Disabled || u.Confirmed {
		return gouncer.ErrUserNotFound
	}
	u.PasswordHash = passwordHash
	u.Confirmed = true
	s.Users[id] = u
	return nil
}

// ResetPassword stores the password hash and ends every session the
// user holds, or returns gouncer.ErrUserNotFound for an unknown or
// disabled one, leaving both untouched.
func (s *Store) ResetPassword(_ context.Context, id uuid.UUID, passwordHash string) error {
	if s.ResetErr != nil {
		return s.ResetErr
	}
	u, ok := s.Users[id]
	if !ok || u.Disabled {
		return gouncer.ErrUserNotFound
	}
	u.PasswordHash = passwordHash
	s.Users[id] = u
	maps.DeleteFunc(s.Sessions, func(_ string, sess gouncer.Session) bool {
		return sess.UserID == id
	})
	return nil
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
