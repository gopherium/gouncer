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

// password is the password of every account EnsureAdmin creates for the suite.
const password = "correct horse battery"

// The privileged roles the guard checks run under.
var (
	admins = gouncer.Roles{"admin"}
	owners = gouncer.Roles{"admin", "owner"}
)

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
	// GrantRoleToRoleless gives role to every account holding none and returns how many it changed.
	GrantRoleToRoleless(ctx context.Context, role string) (int64, error)
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
	{"an account's role reads back through every read", roleOnEveryRead},
	{"SetUserRole writes the role, and an unknown account answers ErrUserNotFound", roleWritten},
	{"the guard refuses to demote the last enabled privileged account and keeps its role", lastDemotion},
	{"the guard refuses to disable the last enabled privileged account and keeps it enabled", lastDisable},
	{"a disabled account holding a privileged role covers no other", disabledCover},
	{"any privileged role covers another, and moving between them needs no cover", twoPrivilegedRoles},
	{"an account holding no privileged role, or an empty privileged set, passes the guard", unguarded},
	{"a guarded disable of a covered account ends its sessions, and a guarded enable needs no cover", coveredDisable},
	{"EnsureAdmin creates a missing account, stamps a roleless one and keeps a held role", ensuredAdmins},
	{"GrantRoleToRoleless gives the role once to every account holding none, disabled ones too", grantedRoleless},
	{"GrantRoleToRoleless refuses the empty role and changes nothing", grantedNothing},
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
	disabled := created(t, fixture, closed(account("bravo@example.com", "Bravo account")))
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

// roleOnEveryRead fails t unless the role an account holds reads back by email, by id, by session and in the list.
func roleOnEveryRead(t testing.TB, fixture Fixture) {
	user := created(t, fixture, holding(account("alpha@example.com", "alpha account"), "admin"))
	session := opened(t, fixture, user, time.Hour)
	bySession, err := fixture.Store.UserBySession(t.Context(), session.TokenHash, now())
	must(t, err, "UserBySession")
	byEmail, err := fixture.Store.UserByEmail(t.Context(), user.Email)
	must(t, err, "UserByEmail")
	listed, err := fixture.Store.ListUsers(t.Context())
	must(t, err, "ListUsers")
	roles := append([]string{bySession.Role, byEmail.Role, byID(t, fixture, user.ID).Role}, rolesOf(listed)...)
	expect(t, slices.Equal(roles, []string{"admin", "admin", "admin", "admin"}), "the reads by session, email, id "+
		"and list carried the roles %q, want admin in each", roles)
}

// roleWritten fails t unless SetUserRole writes a covered account's role and refuses an unknown account.
func roleWritten(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, holding(account("alpha@example.com", "alpha account"), "admin"))
	created(t, fixture, holding(account("bravo@example.com", "Bravo account"), "admin"))
	must(t, fixture.Store.SetUserRole(t.Context(), alpha.ID, "editor", admins), "SetUserRole")
	expect(t, byID(t, fixture, alpha.ID).Role == "editor", "the role after SetUserRole is not editor")
	err := fixture.Store.SetUserRole(t.Context(), firstTwin, "editor", admins)
	expect(t, errors.Is(err, gouncer.ErrUserNotFound), "SetUserRole() of an unknown account error = %v, "+
		"want ErrUserNotFound", err)
}

// lastDemotion fails t unless demoting the last enabled privileged account answers ErrLastPrivileged, keeping the role.
func lastDemotion(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, holding(account("alpha@example.com", "alpha account"), "admin"))
	created(t, fixture, holding(account("bravo@example.com", "Bravo account"), "editor"))
	err := fixture.Store.SetUserRole(t.Context(), alpha.ID, "editor", admins)
	expect(t, errors.Is(err, gouncer.ErrLastPrivileged), "SetUserRole() error = %v, want ErrLastPrivileged", err)
	expect(t, byID(t, fixture, alpha.ID).Role == "admin", "the refused demotion changed the role")
}

// lastDisable fails t unless disabling the last enabled privileged account answers ErrLastPrivileged and keeps it.
func lastDisable(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, holding(account("alpha@example.com", "alpha account"), "admin"))
	created(t, fixture, holding(account("bravo@example.com", "Bravo account"), "editor"))
	err := fixture.Store.SetUserDisabledUnderCover(t.Context(), alpha.ID, true, admins)
	expect(t, errors.Is(err, gouncer.ErrLastPrivileged), "SetUserDisabledUnderCover() error = %v, "+
		"want ErrLastPrivileged", err)
	expect(t, !byID(t, fixture, alpha.ID).Disabled, "the refused disable disabled the account")
}

// disabledCover fails t unless a disabled account holding a privileged role leaves the enabled one uncovered.
func disabledCover(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, holding(account("alpha@example.com", "alpha account"), "admin"))
	created(t, fixture, closed(holding(account("bravo@example.com", "Bravo account"), "admin")))
	demoted := fixture.Store.SetUserRole(t.Context(), alpha.ID, "editor", admins)
	disabled := fixture.Store.SetUserDisabledUnderCover(t.Context(), alpha.ID, true, admins)
	expect(t, errors.Is(demoted, gouncer.ErrLastPrivileged) && errors.Is(disabled, gouncer.ErrLastPrivileged),
		"the demotion and the disable covered by a disabled admin answered %v and %v, want ErrLastPrivileged",
		demoted, disabled)
}

// twoPrivilegedRoles fails t unless each privileged role covers the other and moving between them needs no cover.
func twoPrivilegedRoles(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, holding(account("alpha@example.com", "alpha account"), "admin"))
	bravo := created(t, fixture, holding(account("bravo@example.com", "Bravo account"), "owner"))
	must(t, fixture.Store.SetUserRole(t.Context(), alpha.ID, "editor", owners), "SetUserRole of the covered admin")
	demoted := fixture.Store.SetUserRole(t.Context(), bravo.ID, "editor", owners)
	disabled := fixture.Store.SetUserDisabledUnderCover(t.Context(), bravo.ID, true, owners)
	expect(t, errors.Is(demoted, gouncer.ErrLastPrivileged) && errors.Is(disabled, gouncer.ErrLastPrivileged),
		"the demotion and the disable of the last owner answered %v and %v, want ErrLastPrivileged", demoted, disabled)
	must(t, fixture.Store.SetUserRole(t.Context(), bravo.ID, "admin", owners), "SetUserRole of the last owner to admin")
	expect(t, byID(t, fixture, bravo.ID).Role == "admin", "the move between privileged roles did not land")
}

// unguarded fails t unless an account holding no privileged role, or any account under an empty set, passes the guard.
func unguarded(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, holding(account("alpha@example.com", "alpha account"), "admin"))
	bravo := created(t, fixture, holding(account("bravo@example.com", "Bravo account"), "editor"))
	must(t, fixture.Store.SetUserRole(t.Context(), bravo.ID, "author", admins), "SetUserRole of the editor")
	must(t, fixture.Store.SetUserDisabledUnderCover(t.Context(), bravo.ID, true, admins), "the guarded disable "+
		"of the editor")
	must(t, fixture.Store.SetUserRole(t.Context(), alpha.ID, "editor", nil), "SetUserRole of the lone admin under "+
		"no privileged role")
	must(t, fixture.Store.SetUserDisabledUnderCover(t.Context(), alpha.ID, true, nil), "the guarded disable of the "+
		"former admin under no privileged role")
	got := []gouncer.User{byID(t, fixture, bravo.ID), byID(t, fixture, alpha.ID)}
	expect(t, got[0].Role == "author" && got[0].Disabled && got[1].Role == "editor" && got[1].Disabled,
		"the unguarded writes left %s, want both accounts disabled under author and editor", describedAll(got))
}

// coveredDisable fails t unless a guarded disable of a covered account ends its sessions and a guarded enable lands.
func coveredDisable(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, holding(account("alpha@example.com", "alpha account"), "admin"))
	created(t, fixture, holding(account("bravo@example.com", "Bravo account"), "admin"))
	session := opened(t, fixture, alpha, time.Hour)
	must(t, fixture.Store.SetUserDisabledUnderCover(t.Context(), alpha.ID, true, admins), "the guarded disable")
	_, ended := fixture.Store.UserBySession(t.Context(), session.TokenHash, now())
	disabled := byID(t, fixture, alpha.ID).Disabled
	must(t, fixture.Store.SetUserDisabledUnderCover(t.Context(), alpha.ID, false, admins), "the guarded enable")
	unknown := fixture.Store.SetUserDisabledUnderCover(t.Context(), firstTwin, true, admins)
	expect(t, disabled && !byID(t, fixture, alpha.ID).Disabled, "the guarded disable and enable did not flip the flag")
	expect(t, errors.Is(ended, gouncer.ErrSessionNotFound), "UserBySession() after the guarded disable error = %v, "+
		"want ErrSessionNotFound", ended)
	expect(t, errors.Is(unknown, gouncer.ErrUserNotFound), "the guarded disable of an unknown account error = %v, "+
		"want ErrUserNotFound", unknown)
}

// ensuredAdmins fails t unless EnsureAdmin creates a missing account, stamps a roleless one and keeps a held role.
func ensuredAdmins(t testing.TB, fixture Fixture) {
	created(t, fixture, account("roleless@example.com", "roleless account"))
	created(t, fixture, holding(account("editor@example.com", "editor account"), "editor"))
	made, err := authkit.EnsureAdmin(t.Context(), fixture.Store, "new@example.com", "new account", password, "admin")
	must(t, err, "EnsureAdmin of a new address")
	stamped, err := authkit.EnsureAdmin(t.Context(), fixture.Store, "roleless@example.com", "roleless account",
		password, "admin")
	must(t, err, "EnsureAdmin of the roleless account")
	kept, err := authkit.EnsureAdmin(t.Context(), fixture.Store, "editor@example.com", "editor account", password,
		"admin")
	must(t, err, "EnsureAdmin of the editor")
	expect(t, made && !stamped && !kept, "EnsureAdmin() created %t, %t and %t, want true, false and false", made,
		stamped, kept)
	roles := []string{
		byEmail(t, fixture, "new@example.com").Role, byEmail(t, fixture, "roleless@example.com").Role,
		byEmail(t, fixture, "editor@example.com").Role,
	}
	expect(t, slices.Equal(roles, []string{"admin", "admin", "editor"}), "EnsureAdmin() left the roles %q, "+
		"want admin, admin and editor", roles)
}

// grantedRoleless fails t unless GrantRoleToRoleless gives the role once to every account holding none.
func grantedRoleless(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, account("alpha@example.com", "alpha account"))
	bravo := created(t, fixture, closed(account("bravo@example.com", "Bravo account")))
	charlie := created(t, fixture, holding(account("charlie@example.com", "charlie account"), "editor"))
	granted, err := fixture.Store.GrantRoleToRoleless(t.Context(), "member")
	must(t, err, "GrantRoleToRoleless")
	again, err := fixture.Store.GrantRoleToRoleless(t.Context(), "member")
	must(t, err, "the second GrantRoleToRoleless")
	expect(t, granted == 2 && again == 0, "GrantRoleToRoleless() granted %d, then %d, want 2, then 0", granted, again)
	roles := []string{byID(t, fixture, alpha.ID).Role, byID(t, fixture, bravo.ID).Role, byID(t, fixture, charlie.ID).Role}
	expect(t, slices.Equal(roles, []string{"member", "member", "editor"}), "GrantRoleToRoleless() left the roles %q, "+
		"want member, member and editor", roles)
}

// grantedNothing fails t unless GrantRoleToRoleless refuses the empty role with ErrEmptyRole and changes nothing.
func grantedNothing(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, account("alpha@example.com", "alpha account"))
	granted, err := fixture.Store.GrantRoleToRoleless(t.Context(), "")
	expect(t, errors.Is(err, gouncer.ErrEmptyRole) && granted == 0, "GrantRoleToRoleless(\"\") = %d, %v, "+
		"want 0 and ErrEmptyRole", granted, err)
	expect(t, byID(t, fixture, alpha.ID).Role == "", "the refused grant gave the account a role")
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

// holding returns user under role.
func holding(user gouncer.User, role string) gouncer.User {
	user.Role = role
	return user
}

// closed returns user disabled.
func closed(user gouncer.User) gouncer.User {
	user.Disabled = true
	return user
}

// byID returns the account the store holds under id, ending the check when it holds none.
func byID(t testing.TB, fixture Fixture, id uuid.UUID) gouncer.User {
	t.Helper()
	user, err := fixture.Store.UserByID(t.Context(), id)
	must(t, err, "UserByID "+id.String())
	return user
}

// byEmail returns the account the store holds at email, ending the check when it holds none.
func byEmail(t testing.TB, fixture Fixture, email string) gouncer.User {
	t.Helper()
	user, err := fixture.Store.UserByEmail(t.Context(), email)
	must(t, err, "UserByEmail "+email)
	return user
}

// rolesOf returns the role of each account.
func rolesOf(users []gouncer.User) []string {
	roles := make([]string, 0, len(users))
	for _, user := range users {
		roles = append(roles, user.Role)
	}
	return roles
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
