// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestInviteRouteSendsALinkThatActivatesAndAnswersDelivered(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, "admin", sendingTo(sender))

	wantDelivered(t, inviteMaria(t, routes.admin, "member"))

	sent := sender.invites()
	if got := recipients(sent); !slices.Equal(got, []string{"maria@example.com Maria Perez"}) {
		t.Fatalf("sent to %q, want one invitation to maria@example.com greeting Maria Perez", got)
	}
	wantMariaState(t, store, accountState{name: "Maria Perez", role: "member"})
	wantActivates(t, routes.public, tokenOf(t, sent[0].link))
}

func TestInviteRouteWithoutASenderAnswersALinkThatActivates(t *testing.T) {
	t.Parallel()

	_, routes, token := invitedMaria(t, sendingTo(nil))

	wantActivates(t, routes.public, token)
}

func TestInviteRouteSetsTheTokenAsTheOnlyTokenOfTheAddressQuery(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	cfg := authkit.InvitationsConfig{ActivationURL: "https://example.com/admin/?page=activate&token=stale#password"}
	routes := invitationServer(t, store, "admin", cfg)

	recorder := inviteMaria(t, routes.admin, "member")

	link, _ := decodeBody[map[string]any](t, recorder)["activation_link"].(string)
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", link, err)
	}
	if tokens := parsed.Query()["token"]; len(tokens) != 1 {
		t.Errorf("link = %q, want exactly one token value", link)
	}
	if parsed.Query().Get("token") == "stale" {
		t.Errorf("link = %q, want the fresh token in place of the stale one", link)
	}
	wantActivates(t, routes.public, parsed.Query().Get("token"))
	if parsed.Query().Get("page") != "activate" {
		t.Errorf("link = %q, want the address's own query kept", link)
	}
	if parsed.Fragment != "password" {
		t.Errorf("link = %q, want the fragment kept after the query", link)
	}
}

func TestInviteRouteRefusesARoleTheSourceDoesNotKnow(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, "admin", sendingTo(sender))

	wantRoleUnknown(t, inviteMaria(t, routes.admin, "ghost"))
	wantNoMaria(t, store)
	wantNothingSent(t, sender)
}

func TestInviteRouteRefusesARoleTheActorMayNotGive(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "moderator", sendingTo(nil))

	wantRefused(t, inviteMaria(t, routes.admin, "admin"), http.StatusForbidden, "role_not_grantable")
	wantNoMaria(t, store)
}

func TestInviteRouteRefusesAReaderWithoutAPrivilegedRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "member", sendingTo(nil))

	wantRefused(t, inviteMaria(t, routes.admin, "member"), http.StatusForbidden, "role_insufficient")
	wantNoMaria(t, store)
}

func TestInviteRouteRefusesAnAddressNoAccountCanHold(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", sendingTo(nil))

	recorder := doRequest(t, routes.admin, http.MethodPost, "/api/users/invite",
		`{"email":"not an address","name":"Maria Perez","role":"member"}`)

	wantRefused(t, recorder, http.StatusUnprocessableEntity, "email_invalid")
}

func TestInviteRouteReadsTheNewAccountByItsNormalizedAddress(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, "admin", sendingTo(sender))

	wantDelivered(t, doRequest(t, routes.admin, http.MethodPost, "/api/users/invite",
		`{"email":" Maria@Example.com ","name":"Maria Perez","role":"member"}`))

	if got := recipients(sender.invites()); !slices.Equal(got, []string{"maria@example.com Maria Perez"}) {
		t.Errorf("sent to %q, want one invitation to maria@example.com", got)
	}
}

func TestInviteRouteOfAPendingAddressEndsTheOldLink(t *testing.T) {
	t.Parallel()

	_, routes, first := invitedMaria(t, sendingTo(nil))

	second := wantLink(t, inviteAs(t, routes.admin, "Someone Else", "member"))

	if first == second {
		t.Fatal("second invite answered the first link again")
	}
	wantRefused(t, activate(t, routes.public, first, testPassword), http.StatusUnprocessableEntity, "token_invalid")
	wantActivates(t, routes.public, second)
}

func TestInviteRouteOfAPendingAddressGreetsTheStoredName(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, "admin", sendingTo(sender))
	wantDelivered(t, inviteMaria(t, routes.admin, "member"))

	wantDelivered(t, inviteAs(t, routes.admin, "Someone Else", "member"))

	sent := sender.invites()
	want := []string{"maria@example.com Maria Perez", "maria@example.com Maria Perez"}
	if got := recipients(sent); !slices.Equal(got, want) {
		t.Fatalf("sent to %q, want two invitations greeting the stored name", got)
	}
	if sent[1].link == sent[0].link {
		t.Error("the second invitation carried the first link again")
	}
	wantMariaState(t, store, accountState{name: "Maria Perez", role: "member"})
}

func TestInviteRouteAnswersDeliveredForAHeldAddress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		actor string
		hold  func(*testing.T, *testkit.Store)
	}{
		{name: "an active account", actor: "admin", hold: addActiveMaria},
		{name: "a disabled invitation", actor: "admin", hold: addDisabledPendingMaria},
		{name: "an invitation beyond reach", actor: "moderator", hold: addPendingAdminMaria},
	}
	for _, tc := range cases {
		for _, sender := range []*recordingSender{{}, nil} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				wantHeldAddressUntouched(t, tc.actor, tc.hold, sender)
			})
		}
	}
}

// wantHeldAddressUntouched fails unless inviting Maria's held address answers delivered and sends or changes nothing.
func wantHeldAddressUntouched(
	t *testing.T,
	actor string,
	hold func(*testing.T, *testkit.Store),
	sender *recordingSender,
) {
	t.Helper()
	store := testkit.NewStore()
	routes := invitationServer(t, store, actor, sendingThrough(sender))
	hold(t, store)
	before := storedMaria(t, store)

	wantDelivered(t, inviteAs(t, routes.admin, "Someone Else", "member"))

	wantNothingSent(t, sender)
	wantMariaState(t, store, stateOf(before))
	if storedMaria(t, store).PasswordHash != before.PasswordHash {
		t.Error("the held account's password hash changed")
	}
}

func TestInviteRouteAnswersAFailedDeliveryAndKeepsTheInvitation(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", sendingTo(&recordingSender{err: errRelayDown}))

	wantRefused(t, inviteMaria(t, routes.admin, "member"), http.StatusBadGateway, "delivery_failed")
	wantMariaState(t, store, accountState{name: "Maria Perez", role: "member"})
}

func TestInviteRouteAnswersAFailedReadOfTheNewAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", sendingTo(nil))
	store.LookupErr = errLookupFailed

	wantRefused(t, inviteMaria(t, routes.admin, "member"), http.StatusInternalServerError, "internal")
}

func TestInviteRouteAnswersAFailedReadOfAHeldAddress(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", sendingTo(nil))
	addActiveMaria(t, store)
	store.LookupErr = errLookupFailed

	wantRefused(t, inviteMaria(t, routes.admin, "member"), http.StatusInternalServerError, "internal")
}

func TestInvitationRoutesRefuseAMalformedBody(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", sendingTo(nil))

	for _, target := range []string{"/api/users/invite", "/api/users/invite/resend", "/api/auth/activate"} {
		wantRefused(t, doRequest(t, routes.admin, http.MethodPost, target, "{"), http.StatusBadRequest, "body_malformed")
	}
}
