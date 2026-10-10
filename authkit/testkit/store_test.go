// SPDX-License-Identifier: Apache-2.0

package testkit_test

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/storetest"
	"github.com/gopherium/gouncer/authkit/testkit"
)

// errHook is the error every hook test hands the store.
var errHook = errors.New("forced failure")

// fatalCatcher is a testing.TB that keeps whether a fatal failure ended the call.
type fatalCatcher struct {
	testing.TB
	// fatal reports whether Fatalf ran.
	fatal bool
}

// Helper marks nothing.
func (c *fatalCatcher) Helper() {}

// Fatalf keeps that it ran and ends the call.
func (c *fatalCatcher) Fatalf(string, ...any) {
	c.fatal = true
	runtime.Goexit()
}

func TestTheStoreKeepsTheContract(t *testing.T) {
	t.Parallel()

	storetest.Run(t, func(*testing.T) storetest.Fixture {
		store := testkit.NewStore()
		return storetest.Fixture{
			Store: store,
			Plant: func(_ context.Context, token gouncer.Token) error {
				store.Tokens[string(token.TokenHash)] = token
				return nil
			},
		}
	})
}

func TestEveryErrorHookFailsItsMethod(t *testing.T) {
	t.Parallel()

	id := uuid.Must(uuid.NewV7())
	user := gouncer.User{ID: id, Email: "alpha@example.com", Name: "alpha account"}
	token := gouncer.Token{TokenHash: gouncer.HashToken("token"), UserID: id, Purpose: gouncer.PurposeReset}
	tests := []struct {
		name string
		set  func(store *testkit.Store)
		call func(ctx context.Context, store *testkit.Store) error
	}{
		{"CreateUserErr fails CreateUser", func(s *testkit.Store) { s.CreateUserErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return s.CreateUser(ctx, user) }},
		{"LookupErr fails UserByEmail", func(s *testkit.Store) { s.LookupErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return errOf(s.UserByEmail(ctx, user.Email)) }},
		{"LookupErr fails UserByID", func(s *testkit.Store) { s.LookupErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return errOf(s.UserByID(ctx, id)) }},
		{"CreateSessionErr fails CreateSession", func(s *testkit.Store) { s.CreateSessionErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return s.CreateSession(ctx, gouncer.Session{}) }},
		{"SessionErr fails UserBySession", func(s *testkit.Store) { s.SessionErr = errHook },
			func(ctx context.Context, s *testkit.Store) error {
				return errOf(s.UserBySession(ctx, token.TokenHash, time.Now()))
			}},
		{"DeleteErr fails DeleteSession", func(s *testkit.Store) { s.DeleteErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return s.DeleteSession(ctx, token.TokenHash) }},
		{"ListUsersErr fails ListUsers", func(s *testkit.Store) { s.ListUsersErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return errOf(s.ListUsers(ctx)) }},
		{"SetDisabledErr fails SetUserDisabled", func(s *testkit.Store) { s.SetDisabledErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return s.SetUserDisabled(ctx, id, true) }},
		{"SetDisabledErr fails SetUserDisabledUnderCover", func(s *testkit.Store) { s.SetDisabledErr = errHook },
			func(ctx context.Context, s *testkit.Store) error {
				return s.SetUserDisabledUnderCover(ctx, id, true, nil)
			}},
		{"SetRoleErr fails SetUserRole", func(s *testkit.Store) { s.SetRoleErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return s.SetUserRole(ctx, id, "editor", nil) }},
		{"TokenErr fails CreateToken", func(s *testkit.Store) { s.TokenErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return s.CreateToken(ctx, token, 1) }},
		{"TokenErr fails ReplaceToken", func(s *testkit.Store) { s.TokenErr = errHook },
			func(ctx context.Context, s *testkit.Store) error { return s.ReplaceToken(ctx, token) }},
		{"TokenErr fails ActivateByToken", func(s *testkit.Store) { s.TokenErr = errHook },
			func(ctx context.Context, s *testkit.Store) error {
				return errOf(s.ActivateByToken(ctx, token.TokenHash, time.Now(), "hash"))
			}},
		{"TokenErr fails ResetByToken", func(s *testkit.Store) { s.TokenErr = errHook },
			func(ctx context.Context, s *testkit.Store) error {
				return errOf(s.ResetByToken(ctx, token.TokenHash, time.Now(), "hash"))
			}},
		{"TokenErr fails DeleteExpiredTokens", func(s *testkit.Store) { s.TokenErr = errHook },
			func(ctx context.Context, s *testkit.Store) error {
				return errOf(s.DeleteExpiredTokens(ctx, time.Now()))
			}},
		{"ActivateErr fails ActivateByToken", func(s *testkit.Store) { s.ActivateErr = errHook },
			func(ctx context.Context, s *testkit.Store) error {
				return errOf(s.ActivateByToken(ctx, token.TokenHash, time.Now(), "hash"))
			}},
		{"ResetErr fails ResetByToken", func(s *testkit.Store) { s.ResetErr = errHook },
			func(ctx context.Context, s *testkit.Store) error {
				return errOf(s.ResetByToken(ctx, token.TokenHash, time.Now(), "hash"))
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			tt.set(store)

			if err := tt.call(t.Context(), store); !errors.Is(err, errHook) {
				t.Errorf("error = %v, want the forced failure", err)
			}
		})
	}
}

func TestAddUserEndsTheTestOnInvalidInput(t *testing.T) {
	t.Parallel()

	catcher := &fatalCatcher{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		testkit.NewStore().AddUser(catcher, "not an address", "alpha account", "correct horse battery")
	}()
	<-done

	if !catcher.fatal {
		t.Error("AddUser() with an invalid address went on, want it to end the test")
	}
}

// errOf returns the error of a call, dropping its value.
func errOf[T any](_ T, err error) error {
	return err
}
