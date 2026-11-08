// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"net/http"
	"slices"
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
			})
		}
	}
}

func TestResendRouteHandsNoLinkForAnInvitationPromotedBeyondReachMeanwhile(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	cfg := sendingTo(nil)
	cfg.Admin = authkit.NewAdmin(authkit.AdminConfig{
		Store: promotingReads{store}, Privileged: invitationPrivileged, Roles: ladder(),
	})
	routes := invitationServer(t, store, "moderator", cfg)
	addPendingMariaAs(t, store, "member")

	wantDelivered(t, resendTo(t, routes.admin, "maria@example.com"))
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
