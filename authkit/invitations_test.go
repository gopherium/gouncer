// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

const testActivationURL = "https://example.com/admin/activate"

const testClientAddr = "192.0.2.1"

// invitationPrivileged names the roles the invitation tests treat as administrators.
var invitationPrivileged = gouncer.Roles{"admin", "moderator"}

// invitationRoutes holds the two ways an invitation test reaches the routes.
type invitationRoutes struct {
	// admin carries the signed-in actor's cookie on every request.
	admin http.Handler
	// public carries no cookie, as a link the invited person opens.
	public http.Handler
}

// accountState is what an invitation test compares of an account.
type accountState struct {
	name, role                         string
	confirmed, disabled, holdsPassword bool
}

// stateOf returns what an invitation test compares of the account.
func stateOf(u gouncer.User) accountState {
	return accountState{
		name: u.Name, role: u.Role, confirmed: u.Confirmed, disabled: u.Disabled, holdsPassword: u.PasswordHash != "",
	}
}

// sendingTo returns the invitation settings that deliver through sender to the test activation address.
func sendingTo(sender authkit.InviteSender) authkit.InvitationsConfig {
	return authkit.InvitationsConfig{Sender: sender, ActivationURL: testActivationURL}
}

// sendingThrough returns the settings that deliver through sender, or answer each link when sender is missing.
func sendingThrough(sender *recordingSender) authkit.InvitationsConfig {
	if sender == nil {
		return sendingTo(nil)
	}
	return sendingTo(sender)
}

// limitedBy returns the settings with no sender that count dead links through limiter.
func limitedBy(limiter authkit.LinkLimiter) authkit.InvitationsConfig {
	cfg := sendingTo(nil)
	cfg.Limiter = limiter
	return cfg
}

// invitationServer mounts login, the session and the invitation routes over the ladder, signed in as actorRole.
func invitationServer(
	t *testing.T,
	store *testkit.Store,
	actorRole string,
	cfg authkit.InvitationsConfig,
) invitationRoutes {
	t.Helper()
	addAdaAs(t, store, actorRole)
	h := authkit.New(authkit.Config{Store: store, Privileged: invitationPrivileged})
	if cfg.Invites == nil {
		cfg.Invites = authkit.NewInvites(authkit.InvitesConfig{Store: store, Roles: ladder()})
	}
	cfg.Sessions = h
	if cfg.Admin == nil {
		cfg.Admin = authkit.NewAdmin(authkit.AdminConfig{Store: store, Privileged: invitationPrivileged, Roles: ladder()})
	}
	invitations := authkit.NewInvitations(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.Handle("GET /api/auth/session", h.RequireSession(http.HandlerFunc(h.Session)))
	mux.Handle("POST /api/users/invite", h.RequireSession(http.HandlerFunc(invitations.Invite)))
	mux.Handle("POST /api/users/invite/resend", h.RequireSession(http.HandlerFunc(invitations.Resend)))
	mux.HandleFunc("POST /api/auth/activate", invitations.Activate)
	return invitationRoutes{admin: cookiedServer(mux, loginCookie(t, mux)), public: mux}
}

// invitedMaria returns a fresh store and routes signed in as an admin, with Maria invited and her link's token.
func invitedMaria(t *testing.T, cfg authkit.InvitationsConfig) (*testkit.Store, invitationRoutes, string) {
	t.Helper()
	store := testkit.NewStore()
	routes := invitationServer(t, store, "admin", cfg)
	return store, routes, wantLink(t, inviteMaria(t, routes.admin, "member"))
}

// inviteMaria posts an invitation for Maria Perez holding the role.
func inviteMaria(t *testing.T, srv http.Handler, role string) *httptest.ResponseRecorder {
	t.Helper()
	return inviteAs(t, srv, "Maria Perez", role)
}

// inviteAs posts an invitation for Maria's address under the name, holding the role.
func inviteAs(t *testing.T, srv http.Handler, name, role string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, srv, http.MethodPost, "/api/users/invite",
		`{"email":"maria@example.com","name":"`+name+`","role":"`+role+`"}`)
}

// resendTo posts a resend for the address.
func resendTo(t *testing.T, srv http.Handler, email string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, srv, http.MethodPost, "/api/users/invite/resend", `{"email":"`+email+`"}`)
}

// activation returns the activate request carrying the token and the password, sent from the remote address.
func activation(token, password, remoteAddr string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/auth/activate",
		strings.NewReader(`{"token":"`+token+`","password":"`+password+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remoteAddr
	return request
}

// serve runs the request through the handler and records the response.
func serve(srv http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, request)
	return recorder
}

// activate posts the link's token with the password from the test client address.
func activate(t *testing.T, srv http.Handler, token, password string) *httptest.ResponseRecorder {
	t.Helper()
	return serve(srv, activation(token, password, testClientAddr+":4321"))
}

// activateFrom posts the link's token with the test password from the remote address.
func activateFrom(t *testing.T, srv http.Handler, remoteAddr, token string) {
	t.Helper()
	serve(srv, activation(token, testPassword, remoteAddr))
}

// wantOK fails the test unless the response answers 200.
func wantOK(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
}

// wantActivates fails unless the link's token activates with the test password.
func wantActivates(t *testing.T, srv http.Handler, token string) {
	t.Helper()
	wantOK(t, activate(t, srv, token, testPassword))
}

// wantDelivered fails unless the response reports the link delivered and carries none.
func wantDelivered(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	wantOK(t, recorder)
	if body := decodeBody[map[string]any](t, recorder); !maps.Equal(body, map[string]any{"delivered": true}) {
		t.Errorf("body = %s, want delivered true and no link", recorder.Body)
	}
}

// wantLink fails unless the response hands the link back undelivered, and returns its token.
func wantLink(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	wantOK(t, recorder)
	body := decodeBody[map[string]any](t, recorder)
	if body["delivered"] != false {
		t.Fatalf("body = %s, want delivered false", recorder.Body)
	}
	link, _ := body["activation_link"].(string)
	return tokenOf(t, link)
}

// tokenOf returns the token a link to the test activation address carries.
func tokenOf(t *testing.T, link string) string {
	t.Helper()
	if !strings.HasPrefix(link, testActivationURL+"?token=") {
		t.Fatalf("link = %q, want it to open %s with a token", link, testActivationURL)
	}
	parsed, err := url.Parse(link)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", link, err)
	}
	return parsed.Query().Get("token")
}

// recipients returns the address and the greeted name of each invitation, in the order they were sent.
func recipients(sent []sentInvite) []string {
	named := make([]string, len(sent))
	for i, invite := range sent {
		named[i] = invite.to + " " + invite.name
	}
	return named
}

// wantNothingSent fails when the sender was handed any invitation.
func wantNothingSent(t *testing.T, sender *recordingSender) {
	t.Helper()
	if sent := sender.invites(); len(sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent", sent)
	}
}

// storedMaria returns Maria's stored account.
func storedMaria(t *testing.T, store *testkit.Store) gouncer.User {
	t.Helper()
	maria, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail(maria) error = %v", err)
	}
	return maria
}

// wantMariaState fails unless Maria's stored account stands as want.
func wantMariaState(t *testing.T, store *testkit.Store, want accountState) {
	t.Helper()
	if got := stateOf(storedMaria(t, store)); got != want {
		t.Errorf("stored = %+v, want %+v", got, want)
	}
}

// addPendingMariaAs stores an invited account for Maria holding the role, with no link.
func addPendingMariaAs(t *testing.T, store *testkit.Store, role string) gouncer.User {
	t.Helper()
	maria, err := gouncer.NewInvitedUser("maria@example.com", "Maria Perez")
	if err != nil {
		t.Fatalf("NewInvitedUser() error = %v", err)
	}
	maria.Role = role
	if err := store.CreateUser(t.Context(), maria); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	return maria
}

// pendingMariaLink stores an invitation for Maria holding the role and returns its link's token.
func pendingMariaLink(t *testing.T, store *testkit.Store, role string) string {
	t.Helper()
	invites := authkit.NewInvites(authkit.InvitesConfig{Store: store, Roles: ladder()})
	token, err := invites.Invite(t.Context(), "maria@example.com", "Maria Perez", role)
	if err != nil {
		t.Fatalf("Invite() error = %v", err)
	}
	return token.Token
}

// addDisabledPendingMaria stores a disabled invitation for Maria holding the member role.
func addDisabledPendingMaria(t *testing.T, store *testkit.Store) {
	t.Helper()
	disable(store, addPendingMariaAs(t, store, "member"))
}

// addActiveMaria stores an active account for Maria holding the member role.
func addActiveMaria(t *testing.T, store *testkit.Store) {
	t.Helper()
	addMariaAs(t, store, "member")
}

// addPendingAdminMaria stores an invitation for Maria holding the admin role, beyond a moderator's reach.
func addPendingAdminMaria(t *testing.T, store *testkit.Store) {
	t.Helper()
	addPendingMariaAs(t, store, "admin")
}

// addUnconfirmedMariaWithPassword stores an unconfirmed account for Maria holding the member role and a password.
func addUnconfirmedMariaWithPassword(t *testing.T, store *testkit.Store) {
	t.Helper()
	maria := addPendingMariaAs(t, store, "member")
	if err := maria.SetPassword(testPassword); err != nil {
		t.Fatalf("SetPassword() error = %v", err)
	}
	store.Users[maria.ID] = maria
}

// holdNothing leaves the store as it is.
func holdNothing(*testing.T, *testkit.Store) {}
