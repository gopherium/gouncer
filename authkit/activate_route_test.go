// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestActivateRouteSetsThePasswordConfirmsAndSignsIn(t *testing.T) {
	t.Parallel()

	store, routes, token := invitedMaria(t, sendingTo(nil))

	recorder := activate(t, routes.public, token, "a password Maria chose")

	wantOK(t, recorder)
	if identity := decodeBody[authkit.Identity](t, recorder); identity.Email != "maria@example.com" {
		t.Errorf("identity = %+v, want Maria signed in", identity)
	}
	session := doRequest(t, cookiedServer(routes.public, sessionCookie(t, recorder, testCookieName)),
		http.MethodGet, "/api/auth/session", "")
	wantOK(t, session)
	if identity := decodeBody[authkit.Identity](t, session); identity.Email != "maria@example.com" {
		t.Errorf("session = %s, want Maria's session", session.Body)
	}
	wantMariaState(t, store, accountState{name: "Maria Perez", role: "member", confirmed: true, holdsPassword: true})
	wantOK(t, doLogin(t, routes.public, `{"email":"maria@example.com","password":"a password Maria chose"}`))
}

func TestActivateRouteRefusesADeadLinkAndCountsIt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		kill func(*testing.T, *testkit.Store, invitationRoutes, string) string
	}{
		{name: "an unknown link", kill: func(*testing.T, *testkit.Store, invitationRoutes, string) string {
			return "not-a-link"
		}},
		{name: "a spent link", kill: spendTheLink},
		{name: "an expired link", kill: expireTheLinks},
		{name: "the link of a disabled account", kill: disableTheInvited},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			limiter := newCountingLimiter(5)
			store, routes, token := invitedMaria(t, limitedBy(limiter))
			dead := tc.kill(t, store, routes, token)

			wantRefused(t, activate(t, routes.public, dead, testPassword), http.StatusUnprocessableEntity, "token_invalid")
			if got := limiter.count(testClientAddr); got != 1 {
				t.Errorf("failures for %s = %d, want the dead link counted once", testClientAddr, got)
			}
		})
	}
}

// spendTheLink activates the link once and returns it.
func spendTheLink(t *testing.T, _ *testkit.Store, routes invitationRoutes, token string) string {
	t.Helper()
	wantActivates(t, routes.public, token)
	return token
}

// expireTheLinks moves every stored link's expiry into the past and returns the link.
func expireTheLinks(_ *testing.T, store *testkit.Store, _ invitationRoutes, token string) string {
	for hash, held := range store.Tokens {
		held.ExpiresAt = time.Now().UTC().Add(-time.Minute)
		store.Tokens[hash] = held
	}
	return token
}

// disableTheInvited disables Maria's invited account and returns its link.
func disableTheInvited(t *testing.T, store *testkit.Store, _ invitationRoutes, token string) string {
	t.Helper()
	disable(store, storedMaria(t, store))
	return token
}

func TestActivateRouteRefusesAShortPasswordUncountedAndKeepsTheLink(t *testing.T) {
	t.Parallel()

	limiter := newCountingLimiter(5)
	_, routes, token := invitedMaria(t, limitedBy(limiter))

	wantRefused(t, activate(t, routes.public, token, "short"), http.StatusUnprocessableEntity, "password_too_short")
	wantActivates(t, routes.public, token)
	if got := limiter.count(testClientAddr); got != 0 {
		t.Errorf("failures for %s = %d, want a refused password left uncounted", testClientAddr, got)
	}
}

func TestActivateRouteRefusesAClientOverItsBudgetAndSpendsNothing(t *testing.T) {
	t.Parallel()

	store, routes, token := invitedMaria(t, limitedBy(newCountingLimiter(1)))
	activate(t, routes.public, "not-a-link", testPassword)

	recorder := activate(t, routes.public, token, testPassword)

	wantRefused(t, recorder, http.StatusTooManyRequests, "link_rate_limited")
	if after := recorder.Header().Get("Retry-After"); after != "60" {
		t.Errorf("Retry-After = %q, want %q", after, "60")
	}
	wantMariaState(t, store, accountState{name: "Maria Perez", role: "member"})
}

func TestActivateRouteRoundsAPartialWaitUpToTheNextSecond(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", limitedBy(refusingLimiter{wait: 1500 * time.Millisecond}))

	recorder := activate(t, routes.public, "not-a-link", testPassword)

	wantRefused(t, recorder, http.StatusTooManyRequests, "link_rate_limited")
	if after := recorder.Header().Get("Retry-After"); after != "2" {
		t.Errorf("Retry-After = %q, want %q", after, "2")
	}
}

func TestActivateRouteCountsTheClientTheKeyNames(t *testing.T) {
	t.Parallel()

	limiter := newCountingLimiter(5)
	cfg := limitedBy(limiter)
	cfg.ClientKey = func(*http.Request) string { return "client-a" }
	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", cfg)

	activate(t, routes.public, "not-a-link", testPassword)

	if got := limiter.count("client-a"); got != 1 {
		t.Errorf("failures for client-a = %d, want the dead link counted under the named key", got)
	}
}

func TestActivateRouteKeysEachClientAddressAsRatelimitDoes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, remoteAddr, key string
	}{
		{name: "an IPv4 address", remoteAddr: "192.0.2.9:4321", key: "192.0.2.9"},
		{name: "an address with no port", remoteAddr: "192.0.2.9", key: "192.0.2.9"},
		{name: "an IPv4-mapped address", remoteAddr: "[::ffff:c000:209]:4321", key: "192.0.2.9"},
		{name: "an IPv6 address", remoteAddr: "[2001:db8:1:2::1a]:4321", key: "2001:db8:1:2::"},
		{name: "a scoped IPv6 address", remoteAddr: "[fe80::1a%eth0]:4321", key: "fe80::"},
		{name: "an address that names no IP", remoteAddr: "client.example:4321", key: "client.example"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			limiter := newCountingLimiter(5)
			routes := invitationServer(t, testkit.NewStore(), "admin", limitedBy(limiter))

			activateFrom(t, routes.public, tc.remoteAddr, "not-a-link")

			if got := limiter.count(tc.key); got != 1 {
				t.Errorf("failures for %s = %d, want the dead link counted under it", tc.key, got)
			}
		})
	}
}

func TestActivateRouteCountsOneIPv6NetworkAsOneClient(t *testing.T) {
	t.Parallel()

	limiter := newCountingLimiter(5)
	routes := invitationServer(t, testkit.NewStore(), "admin", limitedBy(limiter))

	activateFrom(t, routes.public, "[2001:db8:1:2::1a]:4321", "not-a-link")
	activateFrom(t, routes.public, "[2001:db8:1:2::2b]:4321", "not-a-link")

	if got := limiter.count("2001:db8:1:2::"); got != 2 {
		t.Errorf("failures for 2001:db8:1:2:: = %d, want both addresses of the /64 counted together", got)
	}
}

func TestActivateRouteAnswersAFailedBudgetCheck(t *testing.T) {
	t.Parallel()

	routes := invitationServer(t, testkit.NewStore(), "admin", limitedBy(failingLimiter{checkErr: errLookupFailed}))

	wantRefused(t, activate(t, routes.public, "not-a-link", testPassword), http.StatusInternalServerError, "internal")
}

func TestActivateRouteAnswersAFailedCountOfADeadLink(t *testing.T) {
	t.Parallel()

	routes := invitationServer(t, testkit.NewStore(), "admin", limitedBy(failingLimiter{recordErr: errLookupFailed}))

	wantRefused(t, activate(t, routes.public, "not-a-link", testPassword), http.StatusInternalServerError, "internal")
}

func TestActivateRouteAnswersAFailedSessionStart(t *testing.T) {
	t.Parallel()

	store, routes, token := invitedMaria(t, sendingTo(nil))
	store.CreateSessionErr = errLookupFailed

	wantRefused(t, activate(t, routes.public, token, testPassword), http.StatusInternalServerError, "internal")
}

func TestActivateRouteAnswersAFailedSessionRead(t *testing.T) {
	t.Parallel()

	store, routes, token := invitedMaria(t, sendingTo(nil))
	store.SessionErr = errLookupFailed

	wantRefused(t, activate(t, routes.public, token, testPassword), http.StatusInternalServerError, "internal")
}

func TestNewInvitationsPanicsWithoutAUsableActivationURL(t *testing.T) {
	t.Parallel()

	for _, address := range []string{"", "https://example.com/%zz"} {
		t.Run(address, func(t *testing.T) {
			t.Parallel()

			defer func() {
				if recover() == nil {
					t.Errorf("NewInvitations accepted the activation address %q", address)
				}
			}()
			authkit.NewInvitations(authkit.InvitationsConfig{Sender: &recordingSender{}, ActivationURL: address})
		})
	}
}
