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

// hookID is the account every hook case asks the store about.
var hookID = uuid.Must(uuid.NewV7())

// hookUser is the account every hook case stores or reads.
var hookUser = gouncer.User{ID: hookID, Email: "alpha@example.com", Name: "alpha account"}

// hookToken is the token every hook case stores or spends.
var hookToken = gouncer.Token{TokenHash: gouncer.HashToken("token"), UserID: hookID, Purpose: gouncer.PurposeReset}

// hookCase is one error hook set on a store and a call it fails.
type hookCase struct {
	name string
	set  func(store *testkit.Store)
	call func(ctx context.Context, store *testkit.Store) error
}

// hookCases pairs every error hook with each method it fails.
var hookCases = []hookCase{
	{"CreateUserErr fails CreateUser", func(s *testkit.Store) { s.CreateUserErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return s.CreateUser(ctx, hookUser) }},
	{"CreateUserErr fails CreateFirstUser", func(s *testkit.Store) { s.CreateUserErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return s.CreateFirstUser(ctx, hookUser) }},
	{"LookupErr fails UserByEmail", func(s *testkit.Store) { s.LookupErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return errOf(s.UserByEmail(ctx, hookUser.Email)) }},
	{"LookupErr fails UserByID", func(s *testkit.Store) { s.LookupErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return errOf(s.UserByID(ctx, hookID)) }},
	{"LookupErr fails UsersByIDs", func(s *testkit.Store) { s.LookupErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return errOf(s.UsersByIDs(ctx, []uuid.UUID{hookID}))
		}},
	{"LookupErr fails HasUsers", func(s *testkit.Store) { s.LookupErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return errOf(s.HasUsers(ctx)) }},
	{"CreateSessionErr fails CreateSession", func(s *testkit.Store) { s.CreateSessionErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return s.CreateSession(ctx, gouncer.Session{}) }},
	{"SessionErr fails UserBySession", func(s *testkit.Store) { s.SessionErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return errOf(s.UserBySession(ctx, hookToken.TokenHash, time.Now()))
		}},
	{"DeleteErr fails DeleteSession", func(s *testkit.Store) { s.DeleteErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return s.DeleteSession(ctx, hookToken.TokenHash) }},
	{"ListUsersErr fails ListUsers", func(s *testkit.Store) { s.ListUsersErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return errOf(s.ListUsers(ctx)) }},
	{"SetDisabledErr fails SetUserDisabled", func(s *testkit.Store) { s.SetDisabledErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return s.SetUserDisabled(ctx, hookID, true) }},
	{"SetDisabledErr fails SetUserDisabledUnderCover", func(s *testkit.Store) { s.SetDisabledErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return s.SetUserDisabledUnderCover(ctx, hookID, true, nil)
		}},
	{"SetDisabledErr fails the guarded disable of the last admin", func(s *testkit.Store) {
		s.SetDisabledErr = errHook
		s.Users[hookID] = gouncer.User{ID: hookID, Email: hookUser.Email, Role: "admin"}
	}, func(ctx context.Context, s *testkit.Store) error {
		return s.SetUserDisabledUnderCover(ctx, hookID, true, gouncer.Roles{"admin"})
	}},
	{"SetRoleErr fails SetUserRole", func(s *testkit.Store) { s.SetRoleErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return s.SetUserRole(ctx, hookID, "editor", nil) }},
	{"SetRoleErr fails SetUserRoleFrom", func(s *testkit.Store) { s.SetRoleErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return s.SetUserRoleFrom(ctx, hookID, "member", "editor", nil)
		}},
	{"SetDisabledErr fails SetUserDisabledFrom", func(s *testkit.Store) { s.SetDisabledErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return s.SetUserDisabledFrom(ctx, hookID, "member", true, nil)
		}},
	{"TokenErr fails CreateToken", func(s *testkit.Store) { s.TokenErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return s.CreateToken(ctx, hookToken, 1) }},
	{"TokenErr fails ReplaceToken", func(s *testkit.Store) { s.TokenErr = errHook },
		func(ctx context.Context, s *testkit.Store) error { return s.ReplaceToken(ctx, hookToken) }},
	{"TokenErr fails ActivateByToken", func(s *testkit.Store) { s.TokenErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return errOf(s.ActivateByToken(ctx, hookToken.TokenHash, time.Now(), "hash"))
		}},
	{"TokenErr fails ResetByToken", func(s *testkit.Store) { s.TokenErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return errOf(s.ResetByToken(ctx, hookToken.TokenHash, time.Now(), "hash"))
		}},
	{"TokenErr fails DeleteExpiredTokens", func(s *testkit.Store) { s.TokenErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return errOf(s.DeleteExpiredTokens(ctx, time.Now()))
		}},
	{"ActivateErr fails ActivateByToken", func(s *testkit.Store) { s.ActivateErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return errOf(s.ActivateByToken(ctx, hookToken.TokenHash, time.Now(), "hash"))
		}},
	{"ResetErr fails ResetByToken", func(s *testkit.Store) { s.ResetErr = errHook },
		func(ctx context.Context, s *testkit.Store) error {
			return errOf(s.ResetByToken(ctx, hookToken.TokenHash, time.Now(), "hash"))
		}},
}

func TestEveryErrorHookFailsItsMethod(t *testing.T) {
	t.Parallel()

	for _, tt := range hookCases {
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
