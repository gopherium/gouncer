// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"net/http"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestResendRouteEndsTheOldLink(t *testing.T) {
	t.Parallel()

	_, routes, first := invitedMaria(t, sendingTo(nil))

	second := wantLink(t, resendTo(t, routes.admin, "maria@example.com"))

	wantRefused(t, activate(t, routes.public, first, testPassword), http.StatusUnprocessableEntity, "token_invalid")
	wantActivates(t, routes.public, second)
}

func TestResendRouteSendsTheNewLinkToTheStoredAddress(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, "admin", sendingTo(sender))
	wantDelivered(t, inviteMaria(t, routes.admin, "member"))

	wantDelivered(t, resendTo(t, routes.admin, " Maria@Example.com "))

	sent := sender.invites()
	want := []string{"maria@example.com Maria Perez", "maria@example.com Maria Perez"}
	if got := recipients(sent); !slices.Equal(got, want) {
		t.Fatalf("sent to %q, want a second invitation to the stored address", got)
	}
	if sent[1].link == sent[0].link {
		t.Error("the resend carried the first link again")
	}
}

func TestResendRouteAnswersDeliveredForAnAddressWithNoInvitationToRenew(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		actor string
		email string
		hold  func(*testing.T, *testkit.Store)
	}{
		{name: "an unknown address", actor: "admin", email: "nobody@example.com", hold: holdNothing},
		{name: "an active account", actor: "admin", email: "ada@example.com", hold: holdNothing},
		{name: "a disabled invitation", actor: "admin", email: "maria@example.com", hold: addDisabledPendingMaria},
		{name: "an invitation beyond reach", actor: "moderator", email: "maria@example.com", hold: addPendingAdminMaria},
	}
	for _, tc := range cases {
		for _, sender := range []*recordingSender{{}, nil} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				store := testkit.NewStore()
				routes := invitationServer(t, store, tc.actor, sendingThrough(sender))
				tc.hold(t, store)

				wantDelivered(t, resendTo(t, routes.admin, tc.email))
				wantNothingSent(t, sender)
				if len(store.Tokens) != 0 {
					t.Errorf("stored tokens = %d, want no link written for an address with nothing to renew", len(store.Tokens))
				}
			})
		}
	}
}

func TestResendRouteHandsNoLinkAndKeepsTheOldOneForAnInvitationPromotedBeyondReachMeanwhile(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	cfg := sendingTo(nil)
	cfg.Admin = authkit.NewAdmin(authkit.AdminConfig{
		Store: promotingReads{store}, Privileged: invitationPrivileged, Roles: ladder(),
	})
	routes := invitationServer(t, store, "moderator", cfg)
	first := pendingMariaLink(t, store, "member")

	wantDelivered(t, resendTo(t, routes.admin, "maria@example.com"))
	wantActivates(t, routes.public, first)
}

func TestResendRouteKeepsTheOldLinkWhenTheFinalLookupFails(t *testing.T) {
	t.Parallel()

	store, routes, _ := readsRunningOut(t, 1)
	first := pendingMariaLink(t, store, "member")

	wantRefused(t, resendTo(t, routes.admin, "maria@example.com"), http.StatusInternalServerError, "internal")
	wantActivates(t, routes.public, first)
}

func TestResendRouteRenewsAfterAResendThatFailedMidway(t *testing.T) {
	t.Parallel()

	store, routes, reads := readsRunningOut(t, 1)
	first := pendingMariaLink(t, store, "member")
	wantRefused(t, resendTo(t, routes.admin, "maria@example.com"), http.StatusInternalServerError, "internal")
	reads.Store(2)

	second := wantLink(t, resendTo(t, routes.admin, "maria@example.com"))

	wantRefused(t, activate(t, routes.public, first, testPassword), http.StatusUnprocessableEntity, "token_invalid")
	wantActivates(t, routes.public, second)
}

func TestResendRouteStoresTheNewLinkWithoutItsSecret(t *testing.T) {
	t.Parallel()

	store, routes, _ := invitedMaria(t, sendingTo(nil))

	wantLink(t, resendTo(t, routes.admin, "maria@example.com"))

	if len(store.Tokens) != 1 {
		t.Fatalf("stored tokens = %d, want the new link alone", len(store.Tokens))
	}
	for _, tok := range store.Tokens {
		if tok.Token != "" {
			t.Error("the new link reached the store carrying its secret, want the hash alone")
		}
	}
}

func TestResendRouteTellsNothingForAnInvitationActivatedMeanwhile(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	cfg := sendingTo(nil)
	cfg.Invites = authkit.NewInvites(authkit.InvitesConfig{Store: activatingStage{store}, Roles: ladder()})
	routes := invitationServer(t, store, "admin", cfg)
	addPendingMariaAs(t, store, "member")

	wantDelivered(t, resendTo(t, routes.admin, "maria@example.com"))
}

// readsRunningOut returns a fresh store and routes signed in as an admin whose account reads fail after count.
func readsRunningOut(t *testing.T, count int32) (*testkit.Store, invitationRoutes, *atomic.Int32) {
	t.Helper()
	store := testkit.NewStore()
	reads := &atomic.Int32{}
	reads.Store(count)
	cfg := sendingTo(nil)
	cfg.Admin = authkit.NewAdmin(authkit.AdminConfig{
		Store: failingLaterReads{Store: store, left: reads}, Privileged: invitationPrivileged, Roles: ladder(),
	})
	return store, invitationServer(t, store, "admin", cfg), reads
}

func TestResendRouteRefusesAReaderWithoutAPrivilegedRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "member", sendingTo(nil))
	addPendingMariaAs(t, store, "member")

	wantRefused(t, resendTo(t, routes.admin, "maria@example.com"), http.StatusForbidden, "role_insufficient")
}

func TestResendRouteAnswersAFailedLookup(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", sendingTo(nil))
	store.LookupErr = errLookupFailed

	wantRefused(t, resendTo(t, routes.admin, "maria@example.com"), http.StatusInternalServerError, "internal")
}

func TestResendRouteAnswersAFailedLinkWrite(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", sendingTo(nil))
	addPendingMariaAs(t, store, "member")
	store.TokenErr = errLookupFailed

	wantRefused(t, resendTo(t, routes.admin, "maria@example.com"), http.StatusInternalServerError, "internal")
}
