// SPDX-License-Identifier: Apache-2.0

// Package storetest is the contract every account store of authkit keeps, run as one test suite.
package storetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
)

// placeholderHash is the password hash every account of the suite carries, which no store reads.
const placeholderHash = "storetest placeholder hash"

// workers is how many goroutines call the store at once.
const workers = 8

// The ids of the two accounts that share one name, in rising order.
var (
	firstTwin  = uuid.MustParse("019a0000-0000-7000-8000-000000000001")
	secondTwin = uuid.MustParse("019a0000-0000-7000-8000-000000000002")
)

// Store is every method of an account store the suite checks.
type Store interface {
	authkit.AdminStore
	authkit.SessionReaper
	// UserByID returns the account with id, disabled or not, or gouncer.ErrUserNotFound.
	UserByID(ctx context.Context, id uuid.UUID) (gouncer.User, error)
	// SetUserDisabled updates whether the account may log in and ends its sessions on disable.
	SetUserDisabled(ctx context.Context, id uuid.UUID, disabled bool) error
}

// Fixture is one fresh, empty store and the hooks the suite drives it with.
type Fixture struct {
	// Store is the store under test.
	Store Store
}

// check is one rule of the contract.
type check struct {
	// name names the rule.
	name string
	// run fails t when the fixture's store breaks the rule.
	run func(t testing.TB, fixture Fixture)
}

// checks are the rules of the contract.
var checks = []check{
	{"a created account reads back by email", readsBackByEmail},
	{"an unknown email answers ErrUserNotFound", unknownEmail},
	{"a taken email answers ErrEmailTaken", takenEmail},
	{"an account reads back by id, disabled or not", readsBackByID},
	{"an unknown id answers ErrUserNotFound", unknownID},
	{"a live session reads back its account", liveSession},
	{"an unknown, expired or disabled session answers ErrSessionNotFound", unusableSessions},
	{"a deleted session is gone and deleting it again succeeds", deletedSession},
	{"the sweep deletes only the expired sessions and counts them", sweptSessions},
	{"an empty store lists no account", emptyList},
	{"accounts list by name regardless of case, then by id, without password hashes", listOrder},
	{"disabling and enabling an account flips its flag", disableFlips},
	{"disabling an account ends its sessions", disableEndsSessions},
	{"disabling an unknown account answers ErrUserNotFound", disableUnknown},
	{"the store takes calls from several goroutines at once", concurrentCalls},
}

// Run checks every rule of the contract as a parallel subtest, each on a fresh fixture that build returns.
func Run(t *testing.T, build func(t *testing.T) Fixture) {
	t.Helper()
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			c.run(t, build(t))
		})
	}
}

// readsBackByEmail fails t unless a created account reads back whole by its email.
func readsBackByEmail(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	got, err := fixture.Store.UserByEmail(t.Context(), user.Email)
	must(t, err, "UserByEmail")
	expect(t, sameUser(got, user), "UserByEmail() = %s, want %s", described(got), described(user))
}

// unknownEmail fails t unless an email no account holds answers gouncer.ErrUserNotFound.
func unknownEmail(t testing.TB, fixture Fixture) {
	_, err := fixture.Store.UserByEmail(t.Context(), "nobody@example.com")
	expect(t, errors.Is(err, gouncer.ErrUserNotFound), "UserByEmail() error = %v, want ErrUserNotFound", err)
}

// takenEmail fails t unless a second account at a taken email answers gouncer.ErrEmailTaken.
func takenEmail(t testing.TB, fixture Fixture) {
	created(t, fixture, account("alpha@example.com", "alpha account"))
	err := fixture.Store.CreateUser(t.Context(), account("alpha@example.com", "second alpha account"))
	expect(t, errors.Is(err, gouncer.ErrEmailTaken), "CreateUser() error = %v, want ErrEmailTaken", err)
}

// readsBackByID fails t unless an account reads back whole by its id, before and after it is disabled.
func readsBackByID(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	got, err := fixture.Store.UserByID(t.Context(), user.ID)
	must(t, err, "UserByID")
	expect(t, sameUser(got, user), "UserByID() = %s, want %s", described(got), described(user))
	must(t, fixture.Store.SetUserDisabled(t.Context(), user.ID, true), "SetUserDisabled")
	got, err = fixture.Store.UserByID(t.Context(), user.ID)
	must(t, err, "UserByID of the disabled account")
	expect(t, got.Disabled, "UserByID() of the disabled account = %s, want it disabled", described(got))
}

// unknownID fails t unless an id no account holds answers gouncer.ErrUserNotFound.
func unknownID(t testing.TB, fixture Fixture) {
	_, err := fixture.Store.UserByID(t.Context(), firstTwin)
	expect(t, errors.Is(err, gouncer.ErrUserNotFound), "UserByID() error = %v, want ErrUserNotFound", err)
}

// liveSession fails t unless a live session reads back the account it belongs to.
func liveSession(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	session := opened(t, fixture, user, time.Hour)
	got, err := fixture.Store.UserBySession(t.Context(), session.TokenHash, now())
	must(t, err, "UserBySession")
	expect(t, sameUser(got, user), "UserBySession() = %s, want %s", described(got), described(user))
}

// unusableSessions fails t unless an unknown token, an expired session and a disabled account's session read nothing.
func unusableSessions(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	disabled := account("bravo@example.com", "Bravo account")
	disabled.Disabled = true
	created(t, fixture, disabled)
	for what, hash := range map[string][]byte{
		"an unknown token":                  gouncer.HashToken("unknown token"),
		"an expired session":                opened(t, fixture, user, -time.Hour).TokenHash,
		"the session of a disabled account": opened(t, fixture, disabled, time.Hour).TokenHash,
	} {
		_, err := fixture.Store.UserBySession(t.Context(), hash, now())
		expect(t, errors.Is(err, gouncer.ErrSessionNotFound), "UserBySession() of %s error = %v, want ErrSessionNotFound",
			what, err)
	}
}

// deletedSession fails t unless a deleted session reads nothing and deleting it again succeeds.
func deletedSession(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	session := opened(t, fixture, user, time.Hour)
	must(t, fixture.Store.DeleteSession(t.Context(), session.TokenHash), "DeleteSession")
	_, err := fixture.Store.UserBySession(t.Context(), session.TokenHash, now())
	expect(t, errors.Is(err, gouncer.ErrSessionNotFound), "UserBySession() after DeleteSession error = %v, "+
		"want ErrSessionNotFound", err)
	err = fixture.Store.DeleteSession(t.Context(), session.TokenHash)
	expect(t, err == nil, "DeleteSession() of a deleted session error = %v, want nil", err)
}

// sweptSessions fails t unless the sweep deletes and counts only the expired sessions, and none the second time.
func sweptSessions(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	opened(t, fixture, user, -time.Hour)
	live := opened(t, fixture, user, time.Hour)
	count, err := fixture.Store.DeleteExpiredSessions(t.Context(), now())
	must(t, err, "DeleteExpiredSessions")
	expect(t, count == 1, "DeleteExpiredSessions() = %d, want the 1 expired session", count)
	_, err = fixture.Store.UserBySession(t.Context(), live.TokenHash, now())
	expect(t, err == nil, "UserBySession() of the live session after the sweep error = %v, want nil", err)
	again, err := fixture.Store.DeleteExpiredSessions(t.Context(), now())
	must(t, err, "the second DeleteExpiredSessions")
	expect(t, again == 0, "the second DeleteExpiredSessions() = %d, want 0", again)
}

// emptyList fails t unless a store holding no account lists none.
func emptyList(t testing.TB, fixture Fixture) {
	listed, err := fixture.Store.ListUsers(t.Context())
	must(t, err, "ListUsers")
	expect(t, len(listed) == 0, "ListUsers() of an empty store = %s, want none", describedAll(listed))
}

// listOrder fails t unless accounts list by name regardless of case, then by id, without their password hashes.
func listOrder(t testing.TB, fixture Fixture) {
	alpha, bravo := account("alpha@example.com", "alpha account"), account("bravo@example.com", "Bravo account")
	charlie := account("charlie@example.com", "charlie account")
	first, second := account("twin-a@example.com", "twin account"), account("twin-b@example.com", "twin account")
	first.ID, second.ID = firstTwin, secondTwin
	for _, user := range []gouncer.User{charlie, second, bravo, first, alpha} {
		created(t, fixture, user)
	}
	listed, err := fixture.Store.ListUsers(t.Context())
	must(t, err, "ListUsers")
	want := []gouncer.User{alpha, bravo, charlie, first, second}
	for i := range want {
		want[i].PasswordHash = ""
	}
	expect(t, slices.EqualFunc(listed, want, sameUser), "ListUsers() = %s, want %s", describedAll(listed),
		describedAll(want))
}

// disableFlips fails t unless disabling and enabling an account flips its flag.
func disableFlips(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	must(t, fixture.Store.SetUserDisabled(t.Context(), user.ID, true), "SetUserDisabled(true)")
	disabled, err := fixture.Store.UserByEmail(t.Context(), user.Email)
	must(t, err, "UserByEmail after the disable")
	must(t, fixture.Store.SetUserDisabled(t.Context(), user.ID, false), "SetUserDisabled(false)")
	enabled, err := fixture.Store.UserByEmail(t.Context(), user.Email)
	must(t, err, "UserByEmail after the enable")
	expect(t, disabled.Disabled && !enabled.Disabled, "the account read disabled %t, then %t, want true, then false",
		disabled.Disabled, enabled.Disabled)
}

// disableEndsSessions fails t unless a disabled account's sessions stay gone after it is enabled again.
func disableEndsSessions(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	session := opened(t, fixture, user, time.Hour)
	must(t, fixture.Store.SetUserDisabled(t.Context(), user.ID, true), "SetUserDisabled(true)")
	must(t, fixture.Store.SetUserDisabled(t.Context(), user.ID, false), "SetUserDisabled(false)")
	_, err := fixture.Store.UserBySession(t.Context(), session.TokenHash, now())
	expect(t, errors.Is(err, gouncer.ErrSessionNotFound), "UserBySession() after a disable and an enable error = %v, "+
		"want ErrSessionNotFound", err)
}

// disableUnknown fails t unless disabling an id no account holds answers gouncer.ErrUserNotFound.
func disableUnknown(t testing.TB, fixture Fixture) {
	err := fixture.Store.SetUserDisabled(t.Context(), firstTwin, true)
	expect(t, errors.Is(err, gouncer.ErrUserNotFound), "SetUserDisabled() error = %v, want ErrUserNotFound", err)
}

// concurrentCalls fails t unless the store serves accounts and sessions to several goroutines at once.
func concurrentCalls(t testing.TB, fixture Fixture) {
	var mu sync.Mutex
	var failures []error
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			err := worker(t.Context(), fixture.Store, account(fmt.Sprintf("worker-%d@example.com", i), "worker account"))
			mu.Lock()
			defer mu.Unlock()
			failures = append(failures, err)
		})
	}
	wg.Wait()
	must(t, errors.Join(failures...), "a concurrent call")
	listed, err := fixture.Store.ListUsers(t.Context())
	must(t, err, "ListUsers")
	expect(t, len(listed) == workers, "ListUsers() after the workers = %d accounts, want %d", len(listed), workers)
}

// worker creates user, opens a session for it and reads both back, answering every failure.
func worker(ctx context.Context, store Store, user gouncer.User) error {
	session := sessionOf(user, time.Hour)
	return errors.Join(
		store.CreateUser(ctx, user), store.CreateSession(ctx, session),
		second(store.UserBySession(ctx, session.TokenHash, now())), second(store.UserByEmail(ctx, user.Email)),
		second(store.ListUsers(ctx)),
	)
}

// second returns the error of a call, dropping its value.
func second[T any](_ T, err error) error {
	return err
}

// account returns an enabled, confirmed account at email named name, created now.
func account(email, name string) gouncer.User {
	return gouncer.User{
		ID: uuid.Must(uuid.NewV7()), Email: email, Name: name, PasswordHash: placeholderHash, Confirmed: true,
		CreatedAt: now(),
	}
}

// created stores user and returns it, ending the check when the store refuses.
func created(t testing.TB, fixture Fixture, user gouncer.User) gouncer.User {
	t.Helper()
	must(t, fixture.Store.CreateUser(t.Context(), user), "CreateUser "+user.Email)
	return user
}

// opened stores a session for user that expires after lasts and returns it, ending the check when the store refuses.
func opened(t testing.TB, fixture Fixture, user gouncer.User, lasts time.Duration) gouncer.Session {
	t.Helper()
	session := sessionOf(user, lasts)
	must(t, fixture.Store.CreateSession(t.Context(), session), "CreateSession")
	return session
}

// sessionOf returns a session for user under a fresh random token, created now and expiring after lasts.
func sessionOf(user gouncer.User, lasts time.Duration) gouncer.Session {
	token := uuid.NewString()
	return gouncer.Session{
		Token: token, TokenHash: gouncer.HashToken(token), UserID: user.ID, CreatedAt: now(), ExpiresAt: now().Add(lasts),
	}
}

// now returns the current UTC time to the microsecond, which every store keeps exactly.
func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

// sameUser reports whether two accounts hold the same values, their times compared by Equal.
func sameUser(a, b gouncer.User) bool {
	return a.ID == b.ID && a.Email == b.Email && a.Name == b.Name && a.PasswordHash == b.PasswordHash &&
		a.Disabled == b.Disabled && a.Confirmed == b.Confirmed && a.Role == b.Role && a.CreatedAt.Equal(b.CreatedAt)
}

// described returns user as a failure message shows it.
func described(user gouncer.User) string {
	return fmt.Sprintf("{%s %s %q disabled:%t confirmed:%t role:%q created:%s hash:%q}", user.ID, user.Email,
		user.Name, user.Disabled, user.Confirmed, user.Role, user.CreatedAt.Format(time.RFC3339Nano), user.PasswordHash)
}

// describedAll returns users as a failure message shows them.
func describedAll(users []gouncer.User) string {
	shown := make([]string, 0, len(users))
	for _, user := range users {
		shown = append(shown, described(user))
	}
	return "[" + strings.Join(shown, " ") + "]"
}

// expect fails t with the message when ok is false.
func expect(t testing.TB, ok bool, format string, args ...any) {
	t.Helper()
	if !ok {
		t.Errorf(format, args...)
	}
}

// must ends the check with a failure naming action when err is not nil.
func must(t testing.TB, err error, action string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}
