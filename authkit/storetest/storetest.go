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

// holdWait is how long a call behind a hold must stay blocked to show that it waits.
const holdWait = 300 * time.Millisecond

// releaseWait is how long a call may take to finish once the hold ends.
const releaseWait = 10 * time.Second

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
	authkit.InviteStore
	authkit.SessionReaper
	authkit.TokenReaper
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
	// Plant stores token as it stands, past the store's own checks, for any account and with any expiry.
	Plant func(ctx context.Context, token gouncer.Token) error
	// Hold opens a write transaction that keeps the account id held, which RunHeld needs and Run never calls.
	Hold func(ctx context.Context, id uuid.UUID) (Held, error)
}

// Held is an open write transaction that keeps one account held until it ends.
type Held interface {
	// Renew deletes the token expired and stores renewed inside the transaction.
	Renew(ctx context.Context, expired, renewed gouncer.Token) error
	// Commit ends the hold and keeps what it wrote.
	Commit(ctx context.Context) error
	// Rollback ends the hold and discards what it wrote.
	Rollback(ctx context.Context) error
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
	{"CreateToken counts only the account's live tokens of the same purpose against the cap", tokenCap},
	{"CreateToken and ReplaceToken refuse a disabled or unknown account with ErrUserNotFound", refusedIssues},
	{"ReplaceToken supersedes the tokens of its purpose and leaves the others", replacedToken},
	{"ReplaceToken refuses an invite for a confirmed account and keeps the invite it holds", refusedReplacement},
	{"ActivateByToken confirms the account, stores the hash, answers its id and spends the invite", activated},
	{"ActivateByToken answers ErrTokenNotFound for an expired invite, a reset token or an unknown one", unusableInvites},
	{"ActivateByToken refuses a confirmed or disabled account with ErrUserNotFound, changing nothing", refusedActivations},
	{"ResetByToken stores the hash and ends every session and every reset token of the account", resetDone},
	{"ResetByToken refuses a disabled account, an expired reset token and an invite", refusedResets},
	{"the token sweep counts the expired tokens and takes the accounts an expired invite strands", sweptTokens},
	{"the token sweep spares an unconfirmed account that still holds a live invite", sparedInvite},
	{"disabling an account, guarded or not, ends its tokens", disableEndsTokens},
	{"concurrent activations spend the invite once", racedActivations},
	{"an issuance racing a guarded disable leaves no token behind", racedIssuance},
	{"a redemption racing a guarded disable settles both", racedRedemption},
	{"concurrent demotions leave exactly one privileged account", racedDemotions},
}

// heldChecks are the rules that need a second transaction holding an account.
var heldChecks = []check{
	{"an issuance waits while the account is held and lands once the hold ends", heldIssuance},
	{"resets queued behind a hold spend the token once", heldResets},
	{"the token sweep spares an invite renewed under a hold beside it", renewedInvite},
}

// Run checks every rule of the contract as a parallel subtest, each on a fresh fixture that build returns.
func Run(t *testing.T, build func(t *testing.T) Fixture) {
	t.Helper()
	runChecks(t, build, checks)
}

// RunHeld checks every rule that needs the fixture's Hold hook, each on a fresh fixture that build returns.
func RunHeld(t *testing.T, build func(t *testing.T) Fixture) {
	t.Helper()
	runChecks(t, build, heldChecks)
}

// runChecks runs each check as a parallel subtest on a fresh fixture that build returns.
func runChecks(t *testing.T, build func(t *testing.T) Fixture, checks []check) {
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
		"an unknown token":                  unknownHash(),
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

// tokenCap fails t unless an expired token and a token of another purpose leave room under the cap.
func tokenCap(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	issued(t, fixture, user, gouncer.PurposeReset, -time.Hour, 3)
	issued(t, fixture, user, gouncer.PurposeInvite, time.Hour, 3)
	for range 3 {
		issued(t, fixture, user, gouncer.PurposeReset, time.Hour, 3)
	}
	err := fixture.Store.CreateToken(t.Context(), tokenFor(user, gouncer.PurposeReset, time.Hour), 3)
	expect(t, errors.Is(err, gouncer.ErrTokenExists), "CreateToken() past the cap error = %v, want ErrTokenExists", err)
}

// refusedIssues fails t unless CreateToken and ReplaceToken refuse a disabled or unknown account.
func refusedIssues(t testing.TB, fixture Fixture) {
	disabled := created(t, fixture, closed(account("alpha@example.com", "alpha account")))
	unknown := account("nobody@example.com", "unknown account")
	for what, err := range map[string]error{
		"CreateToken for a disabled account":  fixture.Store.CreateToken(t.Context(), resetFor(disabled), 1),
		"ReplaceToken for a disabled account": fixture.Store.ReplaceToken(t.Context(), resetFor(disabled)),
		"CreateToken for an unknown account":  fixture.Store.CreateToken(t.Context(), resetFor(unknown), 1),
		"ReplaceToken for an unknown account": fixture.Store.ReplaceToken(t.Context(), resetFor(unknown)),
	} {
		expect(t, errors.Is(err, gouncer.ErrUserNotFound), "%s error = %v, want ErrUserNotFound", what, err)
	}
}

// replacedToken fails t unless ReplaceToken spends the invites the account held and leaves its reset token.
func replacedToken(t testing.TB, fixture Fixture) {
	user := created(t, fixture, invited("alpha@example.com", "alpha account"))
	first := issued(t, fixture, user, gouncer.PurposeInvite, time.Hour, 1)
	reset := issued(t, fixture, user, gouncer.PurposeReset, time.Hour, 1)
	replacement := tokenFor(user, gouncer.PurposeInvite, time.Hour)
	must(t, fixture.Store.ReplaceToken(t.Context(), replacement), "ReplaceToken")
	_, superseded := fixture.Store.ActivateByToken(t.Context(), first.TokenHash, now(), "settled hash")
	_, err := fixture.Store.ResetByToken(t.Context(), reset.TokenHash, now(), "reset hash")
	must(t, err, "ResetByToken of the token of another purpose")
	id, err := fixture.Store.ActivateByToken(t.Context(), replacement.TokenHash, now(), "settled hash")
	must(t, err, "ActivateByToken of the replacement")
	expect(t, errors.Is(superseded, gouncer.ErrTokenNotFound) && id == user.ID, "the superseded invite answered %v "+
		"and the replacement activated %s, want ErrTokenNotFound and %s", superseded, id, user.ID)
}

// refusedReplacement fails t unless ReplaceToken refuses an invite for a confirmed account and keeps its invite.
func refusedReplacement(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	held := issued(t, fixture, user, gouncer.PurposeInvite, time.Hour, 1)
	err := fixture.Store.ReplaceToken(t.Context(), tokenFor(user, gouncer.PurposeInvite, time.Hour))
	_, kept := fixture.Store.ActivateByToken(t.Context(), held.TokenHash, now(), "settled hash")
	expect(t, errors.Is(err, gouncer.ErrUserNotFound), "ReplaceToken() of an invite for a confirmed account "+
		"error = %v, want ErrUserNotFound", err)
	expect(t, errors.Is(kept, gouncer.ErrUserNotFound), "ActivateByToken() of the held invite after the refusal "+
		"error = %v, want ErrUserNotFound from the invite the refusal kept", kept)
}

// activated fails t unless ActivateByToken confirms the account under the hash and spends the invite.
func activated(t testing.TB, fixture Fixture) {
	user := created(t, fixture, invited("alpha@example.com", "alpha account"))
	invite := issued(t, fixture, user, gouncer.PurposeInvite, time.Hour, 1)
	id, err := fixture.Store.ActivateByToken(t.Context(), invite.TokenHash, now(), "settled hash")
	must(t, err, "ActivateByToken")
	_, again := fixture.Store.ActivateByToken(t.Context(), invite.TokenHash, now(), "another hash")
	held := byID(t, fixture, user.ID)
	expect(t, id == user.ID && held.Confirmed && held.PasswordHash == "settled hash", "ActivateByToken() answered %s "+
		"and left %s, want %s confirmed under the settled hash", id, described(held), user.ID)
	expect(t, errors.Is(again, gouncer.ErrTokenNotFound), "the second ActivateByToken() error = %v, "+
		"want ErrTokenNotFound", again)
}

// unusableInvites fails t unless an expired invite, a reset token and an unknown token activate nothing.
func unusableInvites(t testing.TB, fixture Fixture) {
	user := created(t, fixture, invited("alpha@example.com", "alpha account"))
	settled := created(t, fixture, account("bravo@example.com", "Bravo account"))
	invite := issued(t, fixture, user, gouncer.PurposeInvite, time.Hour, 1)
	reset := issued(t, fixture, settled, gouncer.PurposeReset, time.Hour, 1)
	for what, err := range map[string]error{
		"an expired invite": errOf(fixture.Store.ActivateByToken(t.Context(), invite.TokenHash,
			invite.ExpiresAt.Add(time.Second), "settled hash")),
		"a reset token":    errOf(fixture.Store.ActivateByToken(t.Context(), reset.TokenHash, now(), "settled hash")),
		"an unknown token": errOf(fixture.Store.ActivateByToken(t.Context(), unknownHash(), now(), "settled hash")),
	} {
		expect(t, errors.Is(err, gouncer.ErrTokenNotFound), "ActivateByToken() of %s error = %v, want ErrTokenNotFound",
			what, err)
	}
}

// refusedActivations fails t unless an invite for a confirmed or a disabled account activates nothing.
func refusedActivations(t testing.TB, fixture Fixture) {
	settled := created(t, fixture, account("alpha@example.com", "alpha account"))
	disabled := created(t, fixture, closed(invited("bravo@example.com", "Bravo account")))
	held := issued(t, fixture, settled, gouncer.PurposeInvite, time.Hour, 1)
	planted := plantedFor(t, fixture, disabled, gouncer.PurposeInvite)
	_, ofSettled := fixture.Store.ActivateByToken(t.Context(), held.TokenHash, now(), "intruding hash")
	_, ofDisabled := fixture.Store.ActivateByToken(t.Context(), planted.TokenHash, now(), "intruding hash")
	expect(t, errors.Is(ofSettled, gouncer.ErrUserNotFound) && errors.Is(ofDisabled, gouncer.ErrUserNotFound),
		"ActivateByToken() for the confirmed and the disabled account answered %v and %v, want ErrUserNotFound",
		ofSettled, ofDisabled)
	got := []gouncer.User{byID(t, fixture, settled.ID), byID(t, fixture, disabled.ID)}
	expect(t, got[0].PasswordHash == placeholderHash && !got[1].Confirmed, "the refused activations left %s, "+
		"want the first hash kept and the second account unconfirmed", describedAll(got))
}

// resetDone fails t unless ResetByToken stores the hash and ends every session and reset token of the account.
func resetDone(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	session := opened(t, fixture, user, time.Hour)
	family := make([]gouncer.Token, 0, 3)
	for range 3 {
		family = append(family, issued(t, fixture, user, gouncer.PurposeReset, time.Hour, 3))
	}
	id, err := fixture.Store.ResetByToken(t.Context(), family[1].TokenHash, now(), "reset hash")
	must(t, err, "ResetByToken")
	_, ended := fixture.Store.UserBySession(t.Context(), session.TokenHash, now())
	_, before := fixture.Store.ResetByToken(t.Context(), family[0].TokenHash, now(), "another hash")
	_, after := fixture.Store.ResetByToken(t.Context(), family[2].TokenHash, now(), "another hash")
	expect(t, id == user.ID && byID(t, fixture, user.ID).PasswordHash == "reset hash", "ResetByToken() answered %s, "+
		"want %s under the reset hash", id, user.ID)
	expect(t, errors.Is(ended, gouncer.ErrSessionNotFound) && errors.Is(before, gouncer.ErrTokenNotFound) &&
		errors.Is(after, gouncer.ErrTokenNotFound), "after the reset the session answered %v and the sibling tokens "+
		"%v and %v, want ErrSessionNotFound and ErrTokenNotFound", ended, before, after)
}

// refusedResets fails t unless a disabled account, an expired reset token and an invite reset nothing.
func refusedResets(t testing.TB, fixture Fixture) {
	disabled := created(t, fixture, closed(account("alpha@example.com", "alpha account")))
	user := created(t, fixture, account("bravo@example.com", "Bravo account"))
	planted := plantedFor(t, fixture, disabled, gouncer.PurposeReset)
	expired := issued(t, fixture, user, gouncer.PurposeReset, -time.Hour, 1)
	invite := issued(t, fixture, user, gouncer.PurposeInvite, time.Hour, 1)
	_, ofDisabled := fixture.Store.ResetByToken(t.Context(), planted.TokenHash, now(), "intruding hash")
	expect(t, errors.Is(ofDisabled, gouncer.ErrUserNotFound), "ResetByToken() for a disabled account error = %v, "+
		"want ErrUserNotFound", ofDisabled)
	for what, err := range map[string]error{
		"an expired reset token": errOf(fixture.Store.ResetByToken(t.Context(), expired.TokenHash, now(), "reset hash")),
		"an invite":              errOf(fixture.Store.ResetByToken(t.Context(), invite.TokenHash, now(), "reset hash")),
	} {
		expect(t, errors.Is(err, gouncer.ErrTokenNotFound), "ResetByToken() of %s error = %v, want ErrTokenNotFound",
			what, err)
	}
	expect(t, byID(t, fixture, disabled.ID).PasswordHash == placeholderHash, "the refused reset changed the hash")
}

// sweptTokens fails t unless the sweep counts the expired tokens and takes a stranded account with all it held.
func sweptTokens(t testing.TB, fixture Fixture) {
	stranded := created(t, fixture, invited("stranded@example.com", "stranded account"))
	issued(t, fixture, stranded, gouncer.PurposeInvite, -time.Hour, 1)
	issued(t, fixture, stranded, gouncer.PurposeReset, time.Hour, 1)
	opened(t, fixture, stranded, time.Hour)
	settled := created(t, fixture, account("settled@example.com", "settled account"))
	issued(t, fixture, settled, gouncer.PurposeReset, -time.Hour, 1)
	swept, err := fixture.Store.DeleteExpiredTokens(t.Context(), now())
	must(t, err, "DeleteExpiredTokens")
	_, gone := fixture.Store.UserByID(t.Context(), stranded.ID)
	_, kept := fixture.Store.UserByID(t.Context(), settled.ID)
	later, err := fixture.Store.DeleteExpiredTokens(t.Context(), now().Add(2*time.Hour))
	must(t, err, "the later DeleteExpiredTokens")
	expect(t, swept == 2 && later == 0, "the sweeps counted %d, then %d, want the 2 expired tokens, then none left "+
		"of the stranded account", swept, later)
	expect(t, errors.Is(gone, gouncer.ErrUserNotFound) && kept == nil, "after the sweep the stranded account answered "+
		"%v and the settled one %v, want ErrUserNotFound and nil", gone, kept)
}

// sparedInvite fails t unless the sweep keeps an unconfirmed account and the live invite beside its expired one.
func sparedInvite(t testing.TB, fixture Fixture) {
	user := created(t, fixture, invited("alpha@example.com", "alpha account"))
	issued(t, fixture, user, gouncer.PurposeInvite, -time.Hour, 1)
	live := issued(t, fixture, user, gouncer.PurposeInvite, time.Hour, 1)
	swept, err := fixture.Store.DeleteExpiredTokens(t.Context(), now())
	must(t, err, "DeleteExpiredTokens")
	id, err := fixture.Store.ActivateByToken(t.Context(), live.TokenHash, now(), "settled hash")
	expect(t, swept == 1 && err == nil && id == user.ID, "the sweep counted %d and the live invite activated %s with "+
		"%v, want 1 and %s with nil", swept, id, err, user.ID)
}

// disableEndsTokens fails t unless a disable, guarded or not, ends the tokens the account held.
func disableEndsTokens(t testing.TB, fixture Fixture) {
	alpha := created(t, fixture, invited("alpha@example.com", "alpha account"))
	bravo := created(t, fixture, invited("bravo@example.com", "Bravo account"))
	ofAlpha := issued(t, fixture, alpha, gouncer.PurposeInvite, time.Hour, 1)
	ofBravo := issued(t, fixture, bravo, gouncer.PurposeInvite, time.Hour, 1)
	must(t, fixture.Store.SetUserDisabled(t.Context(), alpha.ID, true), "SetUserDisabled(true)")
	must(t, fixture.Store.SetUserDisabled(t.Context(), alpha.ID, false), "SetUserDisabled(false)")
	must(t, fixture.Store.SetUserDisabledUnderCover(t.Context(), bravo.ID, true, admins), "the guarded disable")
	must(t, fixture.Store.SetUserDisabledUnderCover(t.Context(), bravo.ID, false, admins), "the guarded enable")
	_, first := fixture.Store.ActivateByToken(t.Context(), ofAlpha.TokenHash, now(), "settled hash")
	_, guarded := fixture.Store.ActivateByToken(t.Context(), ofBravo.TokenHash, now(), "settled hash")
	expect(t, errors.Is(first, gouncer.ErrTokenNotFound) && errors.Is(guarded, gouncer.ErrTokenNotFound), "the invites "+
		"after the plain and the guarded disable answered %v and %v, want ErrTokenNotFound", first, guarded)
}

// racedActivations fails t unless invites redeemed by several goroutines at once confirm the account once.
func racedActivations(t testing.TB, fixture Fixture) {
	user := created(t, fixture, invited("alpha@example.com", "alpha account"))
	invite := issued(t, fixture, user, gouncer.PurposeInvite, time.Hour, 1)
	answers := raced(func(int) error {
		return errOf(fixture.Store.ActivateByToken(t.Context(), invite.TokenHash, now(), "settled hash"))
	})
	won, refused := tally(answers, gouncer.ErrTokenNotFound)
	expect(t, won == 1 && refused == workers-1, "the racing activations answered %v, want one nil and "+
		"ErrTokenNotFound for the rest", answers)
	expect(t, byID(t, fixture, user.ID).PasswordHash == "settled hash", "the account lost the settled hash")
}

// racedIssuance fails t unless a token issued beside a guarded disable is gone once the account is enabled again.
func racedIssuance(t testing.TB, fixture Fixture) {
	user := created(t, fixture, invited("alpha@example.com", "alpha account"))
	invite := tokenFor(user, gouncer.PurposeInvite, time.Hour)
	issuedErr, disabled := paired(
		func() error { return fixture.Store.CreateToken(t.Context(), invite, 1) },
		func() error { return fixture.Store.SetUserDisabledUnderCover(t.Context(), user.ID, true, admins) },
	)
	expect(t, disabled == nil && (issuedErr == nil || errors.Is(issuedErr, gouncer.ErrUserNotFound)), "the issuance "+
		"and the disable answered %v and %v, want nil or ErrUserNotFound, then nil", issuedErr, disabled)
	must(t, fixture.Store.SetUserDisabledUnderCover(t.Context(), user.ID, false, admins), "the guarded enable")
	_, err := fixture.Store.ActivateByToken(t.Context(), invite.TokenHash, now(), "intruding hash")
	expect(t, errors.Is(err, gouncer.ErrTokenNotFound), "ActivateByToken() after the disable error = %v, "+
		"want ErrTokenNotFound", err)
}

// racedRedemption fails t unless a redemption racing a guarded disable ends in a refusal or a success, never a fault.
func racedRedemption(t testing.TB, fixture Fixture) {
	user := created(t, fixture, invited("alpha@example.com", "alpha account"))
	invite := issued(t, fixture, user, gouncer.PurposeInvite, time.Hour, 1)
	redeemed, disabled := paired(
		func() error {
			return errOf(fixture.Store.ActivateByToken(t.Context(), invite.TokenHash, now(), "settled hash"))
		},
		func() error { return fixture.Store.SetUserDisabledUnderCover(t.Context(), user.ID, true, admins) },
	)
	settled := redeemed == nil || errors.Is(redeemed, gouncer.ErrUserNotFound) ||
		errors.Is(redeemed, gouncer.ErrTokenNotFound)
	expect(t, settled && disabled == nil, "the redemption and the disable answered %v and %v, want nil or a "+
		"gouncer refusal, then nil", redeemed, disabled)
}

// racedDemotions fails t unless demotions of every privileged account at once leave exactly one of them.
func racedDemotions(t testing.TB, fixture Fixture) {
	ids := make([]uuid.UUID, workers)
	for i := range ids {
		user := holding(account(fmt.Sprintf("admin-%d@example.com", i), "admin account"), "admin")
		ids[i] = created(t, fixture, user).ID
	}
	answers := raced(func(i int) error {
		return fixture.Store.SetUserRole(t.Context(), ids[i], "editor", admins)
	})
	won, refused := tally(answers, gouncer.ErrLastPrivileged)
	listed, err := fixture.Store.ListUsers(t.Context())
	must(t, err, "ListUsers")
	roles := rolesOf(listed)
	left := len(roles) - len(slices.DeleteFunc(slices.Clone(roles), isAdmin))
	expect(t, won == workers-1 && refused == 1 && left == 1, "the racing demotions answered %v and left the roles %q, "+
		"want ErrLastPrivileged once and one admin left", answers, roles)
}

// heldIssuance fails t unless CreateToken waits while the account is held and lands once the hold ends.
func heldIssuance(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	hold := held(t, fixture, user.ID)
	issuedErr := make(chan error, 1)
	go func() { issuedErr <- fixture.Store.CreateToken(t.Context(), resetFor(user), 1) }()
	early, arrived := settle(issuedErr, holdWait)
	expect(t, !arrived, "CreateToken() returned while the account was held, want it to wait")
	must(t, hold.Rollback(t.Context()), "ending the hold")
	err, done := later(issuedErr, early, arrived)
	expect(t, done && err == nil, "CreateToken() after the hold finished %t with %v, want true with nil", done, err)
}

// heldResets fails t unless resets queued behind a hold on the account spend the reset token once.
func heldResets(t testing.TB, fixture Fixture) {
	user := created(t, fixture, account("alpha@example.com", "alpha account"))
	reset := issued(t, fixture, user, gouncer.PurposeReset, time.Hour, 1)
	hold := held(t, fixture, user.ID)
	queued := make(chan error, workers)
	for range workers {
		go func() {
			queued <- errOf(fixture.Store.ResetByToken(t.Context(), reset.TokenHash, now(), "reset hash"))
		}()
	}
	first, early := settle(queued, holdWait)
	expect(t, !early, "a reset finished while the account was held, want every one to wait")
	must(t, hold.Rollback(t.Context()), "ending the hold")
	answers := gathered(queued, workers, first, early, releaseWait)
	won, refused := tally(answers, gouncer.ErrTokenNotFound)
	expect(t, len(answers) == workers && won == 1 && refused == workers-1, "the queued resets answered %v, "+
		"want one nil and ErrTokenNotFound for the other %d", answers, workers-1)
	expect(t, byID(t, fixture, user.ID).PasswordHash == "reset hash", "the account lost the reset hash")
}

// renewedInvite fails t unless the sweep waiting on a hold spares the account whose invite the hold renews.
func renewedInvite(t testing.TB, fixture Fixture) {
	user := created(t, fixture, invited("alpha@example.com", "alpha account"))
	expired := issued(t, fixture, user, gouncer.PurposeInvite, -time.Hour, 1)
	hold := held(t, fixture, user.ID)
	swept := make(chan error, 1)
	go func() { swept <- errOf(fixture.Store.DeleteExpiredTokens(t.Context(), now())) }()
	early, arrived := settle(swept, holdWait)
	expect(t, !arrived, "the sweep finished while the account was held, want it to wait")
	renewed := tokenFor(user, gouncer.PurposeInvite, time.Hour)
	must(t, hold.Renew(t.Context(), expired, renewed), "renewing the invite under the hold")
	must(t, hold.Commit(t.Context()), "committing the hold")
	err, done := later(swept, early, arrived)
	expect(t, done && err == nil, "the sweep after the hold finished %t with %v, want true with nil", done, err)
	id, err := fixture.Store.ActivateByToken(t.Context(), renewed.TokenHash, now(), "settled hash")
	expect(t, err == nil && id == user.ID, "the renewed invite activated %s with %v, want %s with nil", id, err, user.ID)
}

// worker creates user, opens a session for it and reads both back, answering every failure.
func worker(ctx context.Context, store Store, user gouncer.User) error {
	session := sessionOf(user, time.Hour)
	return errors.Join(
		store.CreateUser(ctx, user), store.CreateSession(ctx, session),
		errOf(store.UserBySession(ctx, session.TokenHash, now())), errOf(store.UserByEmail(ctx, user.Email)),
		errOf(store.ListUsers(ctx)),
	)
}

// raced runs body for each of the workers on goroutines released at once and returns every answer in order.
func raced(body func(i int) error) []error {
	answers := make([]error, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			<-start
			answers[i] = body(i)
		})
	}
	close(start)
	wg.Wait()
	return answers
}

// paired runs first and second on two goroutines released at once and returns both answers.
func paired(first, second func() error) (error, error) {
	var firstErr, secondErr error
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		firstErr = first()
	})
	wg.Go(func() {
		<-start
		secondErr = second()
	})
	close(start)
	wg.Wait()
	return firstErr, secondErr
}

// tally counts the answers that are nil and the ones that match want.
func tally(answers []error, want error) (won, refused int) {
	for _, err := range answers {
		switch {
		case err == nil:
			won++
		case errors.Is(err, want):
			refused++
		}
	}
	return won, refused
}

// isAdmin reports whether role is admin.
func isAdmin(role string) bool {
	return role == "admin"
}

// held opens the fixture's hold on the account id and rolls it back when the check ends, ending the check on failure.
func held(t testing.TB, fixture Fixture, id uuid.UUID) Held {
	t.Helper()
	opened, err := fixture.Hold(t.Context(), id)
	must(t, err, "Hold")
	hold := &endOnce{Held: opened}
	t.Cleanup(func() { _ = hold.Rollback(context.Background()) })
	return hold
}

// endOnce is a hold that passes on only the first Commit or Rollback.
type endOnce struct {
	Held
	// once guards the end of the hold.
	once sync.Once
}

// Commit ends the hold unless it has ended already.
func (h *endOnce) Commit(ctx context.Context) error {
	return h.end(func() error { return h.Held.Commit(ctx) })
}

// Rollback ends the hold unless it has ended already.
func (h *endOnce) Rollback(ctx context.Context) error {
	return h.end(func() error { return h.Held.Rollback(ctx) })
}

// end runs finish when no end call came before and returns what it answered.
func (h *endOnce) end(finish func() error) error {
	var err error
	h.once.Do(func() { err = finish() })
	return err
}

// settle returns what arrives on ch within d and whether anything arrived.
func settle[T any](ch <-chan T, d time.Duration) (T, bool) {
	select {
	case value := <-ch:
		return value, true
	case <-time.After(d):
		var zero T
		return zero, false
	}
}

// later returns the answer that arrived early, or else what arrives on ch within releaseWait.
func later[T any](ch <-chan T, early T, arrived bool) (T, bool) {
	if arrived {
		return early, true
	}
	return settle(ch, releaseWait)
}

// gathered returns the n answers that reach ch within wait, the first being the one that came early.
func gathered(ch <-chan error, n int, first error, early bool, wait time.Duration) []error {
	var answers []error
	if early {
		answers = append(answers, first)
	}
	deadline := time.After(wait)
	for len(answers) < n {
		select {
		case answer := <-ch:
			answers = append(answers, answer)
		case <-deadline:
			return answers
		}
	}
	return answers
}

// errOf returns the error of a call, dropping its value.
func errOf[T any](_ T, err error) error {
	return err
}

// account returns an enabled, confirmed account at email named name, created now.
func account(email, name string) gouncer.User {
	return gouncer.User{
		ID: uuid.Must(uuid.NewV7()), Email: email, Name: name, PasswordHash: placeholderHash, Confirmed: true,
		CreatedAt: now(),
	}
}

// invited returns an enabled account at email named name that has not set a password yet.
func invited(email, name string) gouncer.User {
	user := account(email, name)
	user.Confirmed, user.PasswordHash = false, ""
	return user
}

// tokenFor returns a token for user under purpose with a fresh random secret, created now and expiring after lasts.
func tokenFor(user gouncer.User, purpose gouncer.TokenPurpose, lasts time.Duration) gouncer.Token {
	secret := uuid.NewString()
	return gouncer.Token{
		Token: secret, TokenHash: gouncer.HashToken(secret), UserID: user.ID, Purpose: purpose, CreatedAt: now(),
		ExpiresAt: now().Add(lasts),
	}
}

// resetFor returns a live reset token for user.
func resetFor(user gouncer.User) gouncer.Token {
	return tokenFor(user, gouncer.PurposeReset, time.Hour)
}

// unknownHash returns the hash of a token no store holds.
func unknownHash() []byte {
	return gouncer.HashToken("unknown token")
}

// issued stores a token for user through CreateToken under the cap live, ending the check when the store refuses.
func issued(
	t testing.TB, fixture Fixture, user gouncer.User, purpose gouncer.TokenPurpose, lasts time.Duration, live int,
) gouncer.Token {
	t.Helper()
	token := tokenFor(user, purpose, lasts)
	must(t, fixture.Store.CreateToken(t.Context(), token, live), "CreateToken "+string(purpose))
	return token
}

// plantedFor stores a live token for user through the fixture's Plant hook, ending the check when it fails.
func plantedFor(t testing.TB, fixture Fixture, user gouncer.User, purpose gouncer.TokenPurpose) gouncer.Token {
	t.Helper()
	token := tokenFor(user, purpose, time.Hour)
	must(t, fixture.Plant(t.Context(), token), "Plant "+string(purpose))
	return token
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
