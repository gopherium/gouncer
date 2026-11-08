// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

var (
	_ authkit.InviteSender = (*recordingSender)(nil)
	_ authkit.LinkLimiter  = (*countingLimiter)(nil)
	_ authkit.LinkLimiter  = failingLimiter{}
)

const testActivationURL = "https://example.com/admin/activate"

const testClientAddr = "192.0.2.1"

// errRelayDown is the failure a sender answers when its relay refuses the mail.
var errRelayDown = errors.New("relay down")

// sentInvite is one invitation a sender was handed.
type sentInvite struct {
	to, name, link string
}

// recordingSender records every invitation it is handed, failing each one while err is set.
type recordingSender struct {
	mu   sync.Mutex
	sent []sentInvite
	err  error
}

// SendInvite records the invitation and answers the sender's failure.
func (s *recordingSender) SendInvite(_ context.Context, to, name, link string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, sentInvite{to: to, name: name, link: link})
	return s.err
}

// invites returns every invitation the sender was handed.
func (s *recordingSender) invites() []sentInvite {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sentInvite{}, s.sent...)
}

// countingLimiter refuses a key once it counted limit failures for it.
type countingLimiter struct {
	mu       sync.Mutex
	limit    int
	failures map[string]int
}

// newCountingLimiter returns a limiter refusing a key after limit failures.
func newCountingLimiter(limit int) *countingLimiter {
	return &countingLimiter{limit: limit, failures: map[string]int{}}
}

// Check reports whether key holds fewer failures than the limit, and a minute's wait when it does not.
func (l *countingLimiter) Check(key string) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failures[key] >= l.limit {
		return false, time.Minute, nil
	}
	return true, 0, nil
}

// RecordFailure counts one failure for key.
func (l *countingLimiter) RecordFailure(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key]++
	return nil
}

// count returns the failures counted for key.
func (l *countingLimiter) count(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.failures[key]
}

// deliveryBody is the delivery report an invite or a resend answers.
type deliveryBody struct {
	Delivered      *bool   `json:"delivered"`
	ActivationLink *string `json:"activation_link"`
}

// invitationRoutes holds the two ways an invitation test reaches the routes.
type invitationRoutes struct {
	// admin carries the signed-in actor's cookie on every request.
	admin http.Handler
	// public carries no cookie, as a link the invited person opens.
	public http.Handler
}

// sendingTo returns the invitation settings that deliver through sender to the test activation address.
func sendingTo(sender authkit.InviteSender) authkit.InvitationsConfig {
	return authkit.InvitationsConfig{Sender: sender, ActivationURL: testActivationURL}
}

// failingLimiter answers its errors from Check and RecordFailure, admitting every key.
type failingLimiter struct {
	checkErr, recordErr error
}

// Check admits every key, answering the limiter's check failure.
func (l failingLimiter) Check(string) (bool, time.Duration, error) {
	return true, 0, l.checkErr
}

// RecordFailure answers the limiter's record failure.
func (l failingLimiter) RecordFailure(string) error {
	return l.recordErr
}

// invitationServer mounts login, the session and the invitation routes, signed in as Ada holding actorRole.
func invitationServer(
	t *testing.T,
	store *testkit.Store,
	roles authkit.RoleSource,
	actorRole string,
	cfg authkit.InvitationsConfig,
) invitationRoutes {
	t.Helper()
	addAdaAs(t, store, actorRole)
	privileged := gouncer.Roles{"admin", "moderator"}
	h := authkit.New(authkit.Config{Store: store, Privileged: privileged})
	cfg.Invites = authkit.NewInvites(authkit.InvitesConfig{Store: store, Roles: roles})
	cfg.Sessions = h
	cfg.Admin = authkit.NewAdmin(authkit.AdminConfig{Store: store, Privileged: privileged, Roles: roles})
	invitations := authkit.NewInvitations(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.Handle("GET /api/auth/session", h.RequireSession(http.HandlerFunc(h.Session)))
	mux.Handle("POST /api/users/invite", h.RequireSession(http.HandlerFunc(invitations.Invite)))
	mux.Handle("POST /api/users/invite/resend", h.RequireSession(http.HandlerFunc(invitations.Resend)))
	mux.HandleFunc("POST /api/auth/activate", invitations.Activate)
	return invitationRoutes{admin: cookiedServer(mux, loginCookie(t, mux)), public: mux}
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

// activate posts the link's token with the password from the test client address.
func activate(t *testing.T, srv http.Handler, token, password string) *httptest.ResponseRecorder {
	t.Helper()
	return activateFrom(t, srv, testClientAddr+":4321", token, password)
}

// activateFrom posts the link's token with the password from the remote address.
func activateFrom(t *testing.T, srv http.Handler, remoteAddr, token, password string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/activate",
		strings.NewReader(`{"token":"`+token+`","password":"`+password+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remoteAddr
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, request)
	return recorder
}

// wantDelivered fails unless the response reports the link delivered and carries none.
func wantDelivered(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
	body := decodeBody[deliveryBody](t, recorder)
	if body.Delivered == nil || !*body.Delivered || body.ActivationLink != nil {
		t.Errorf("body = %s, want delivered true and no link", recorder.Body)
	}
}

// wantLink fails unless the response hands the link back undelivered, and returns its token.
func wantLink(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
	body := decodeBody[deliveryBody](t, recorder)
	if body.Delivered == nil || *body.Delivered || body.ActivationLink == nil {
		t.Fatalf("body = %s, want delivered false with a link", recorder.Body)
	}
	return tokenOf(t, *body.ActivationLink)
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

// storedMaria returns Maria's stored account.
func storedMaria(t *testing.T, store *testkit.Store) gouncer.User {
	t.Helper()
	maria, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail(maria) error = %v", err)
	}
	return maria
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

func TestInviteRouteSendsTheLinkAndAnswersDelivered(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(sender))

	wantDelivered(t, inviteMaria(t, routes.admin, "member"))

	sent := sender.invites()
	if len(sent) != 1 || sent[0].to != "maria@example.com" || sent[0].name != "Maria Perez" {
		t.Fatalf("sent = %+v, want one invitation to maria@example.com greeting Maria Perez", sent)
	}
	tokenOf(t, sent[0].link)
	maria := storedMaria(t, store)
	if maria.Confirmed || maria.PasswordHash != "" || maria.Role != "member" {
		t.Errorf("stored = %+v, want an unconfirmed member with no password", maria)
	}
}

func TestInviteRouteWithoutASenderAnswersALinkThatActivates(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))

	token := wantLink(t, inviteMaria(t, routes.admin, "member"))

	if recorder := activate(t, routes.public, token, testPassword); recorder.Code != http.StatusOK {
		t.Errorf("activate status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
}

func TestInviteRouteAddsTheTokenAfterAQueryTheAddressHolds(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	cfg := authkit.InvitationsConfig{ActivationURL: "https://example.com/admin/?page=activate"}
	routes := invitationServer(t, store, ladder(), "admin", cfg)

	recorder := inviteMaria(t, routes.admin, "member")

	link := decodeBody[deliveryBody](t, recorder).ActivationLink
	if link == nil || !strings.HasPrefix(*link, "https://example.com/admin/?page=activate&token=") {
		t.Errorf("body = %s, want the token added after the address's own query", recorder.Body)
	}
}

func TestInviteRouteRefusesARoleTheSourceDoesNotKnow(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(sender))

	wantRoleUnknown(t, inviteMaria(t, routes.admin, "ghost"))
	wantNoMaria(t, store)
	if sent := sender.invites(); len(sent) != 0 {
		t.Errorf("sent = %+v, want nothing sent", sent)
	}
}

func TestInviteRouteRefusesARoleTheActorMayNotGive(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "moderator", sendingTo(nil))

	wantRefused(t, inviteMaria(t, routes.admin, "admin"), http.StatusForbidden, "role_not_grantable")
	wantNoMaria(t, store)
}

func TestInviteRouteRefusesAReaderWithoutAPrivilegedRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "member", sendingTo(nil))

	wantRefused(t, inviteMaria(t, routes.admin, "member"), http.StatusForbidden, "role_insufficient")
	wantNoMaria(t, store)
}

func TestInviteRouteRefusesAnAddressNoAccountCanHold(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))

	recorder := doRequest(t, routes.admin, http.MethodPost, "/api/users/invite",
		`{"email":"not an address","name":"Maria Perez","role":"member"}`)

	wantRefused(t, recorder, http.StatusUnprocessableEntity, "email_invalid")
}

func TestInviteRouteOfAPendingAddressEndsTheOldLink(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	first := wantLink(t, inviteMaria(t, routes.admin, "member"))

	second := wantLink(t, inviteAs(t, routes.admin, "Someone Else", "member"))

	if first == second {
		t.Fatal("second invite answered the first link again")
	}
	wantRefused(t, activate(t, routes.public, first, testPassword), http.StatusUnprocessableEntity, "token_invalid")
	if recorder := activate(t, routes.public, second, testPassword); recorder.Code != http.StatusOK {
		t.Errorf("activate status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
}

func TestInviteRouteOfAPendingAddressGreetsTheStoredName(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(sender))
	wantDelivered(t, inviteMaria(t, routes.admin, "member"))

	wantDelivered(t, inviteAs(t, routes.admin, "Someone Else", "member"))

	sent := sender.invites()
	if len(sent) != 2 || sent[1].name != "Maria Perez" || sent[1].link == sent[0].link {
		t.Errorf("sent = %+v, want a second, fresh link greeting the stored name", sent)
	}
	if name := storedMaria(t, store).Name; name != "Maria Perez" {
		t.Errorf("stored name = %q, want it kept", name)
	}
}

func TestInviteRouteAnswersDeliveredForAHeldAddress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		actor string
		hold  func(*testing.T, *testkit.Store) gouncer.User
	}{
		{name: "an active account", actor: "admin", hold: func(t *testing.T, store *testkit.Store) gouncer.User {
			t.Helper()
			return addMariaAs(t, store, "member")
		}},
		{name: "a disabled invitation", actor: "admin", hold: func(t *testing.T, store *testkit.Store) gouncer.User {
			t.Helper()
			maria := addPendingMariaAs(t, store, "member")
			disable(store, maria)
			maria.Disabled = true
			return maria
		}},
		{name: "an invitation beyond reach", actor: "moderator",
			hold: func(t *testing.T, store *testkit.Store) gouncer.User {
				t.Helper()
				return addPendingMariaAs(t, store, "admin")
			}},
	}
	for _, tc := range cases {
		for _, sender := range []*recordingSender{{}, nil} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				store := testkit.NewStore()
				cfg := sendingTo(nil)
				if sender != nil {
					cfg = sendingTo(sender)
				}
				routes := invitationServer(t, store, ladder(), tc.actor, cfg)
				held := tc.hold(t, store)

				wantDelivered(t, inviteAs(t, routes.admin, "Someone Else", "member"))
				if sender != nil && len(sender.invites()) != 0 {
					t.Errorf("sent = %+v, want nothing sent", sender.invites())
				}
				if after := storedMaria(t, store); after.Name != held.Name || after.Role != held.Role ||
					after.Disabled != held.Disabled || after.Confirmed != held.Confirmed {
					t.Errorf("stored = %+v, want the held account unchanged from %+v", after, held)
				}
			})
		}
	}
}

func TestInviteRouteAnswersAFailedDeliveryAndKeepsTheInvitation(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{err: errRelayDown}
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(sender))

	wantRefused(t, inviteMaria(t, routes.admin, "member"), http.StatusBadGateway, "delivery_failed")
	if maria := storedMaria(t, store); maria.Confirmed {
		t.Errorf("stored = %+v, want the account still invited", maria)
	}
}

func TestResendRouteEndsTheOldLink(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	first := wantLink(t, inviteMaria(t, routes.admin, "member"))

	second := wantLink(t, resendTo(t, routes.admin, "maria@example.com"))

	wantRefused(t, activate(t, routes.public, first, testPassword), http.StatusUnprocessableEntity, "token_invalid")
	if recorder := activate(t, routes.public, second, testPassword); recorder.Code != http.StatusOK {
		t.Errorf("activate status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
}

func TestResendRouteSendsTheNewLink(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(sender))
	wantDelivered(t, inviteMaria(t, routes.admin, "member"))

	wantDelivered(t, resendTo(t, routes.admin, " Maria@Example.com "))

	sent := sender.invites()
	if len(sent) != 2 || sent[1].to != "maria@example.com" || sent[1].link == sent[0].link {
		t.Errorf("sent = %+v, want a second, fresh link to the stored address", sent)
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
		{name: "an unknown address", actor: "admin", email: "nobody@example.com",
			hold: func(*testing.T, *testkit.Store) {}},
		{name: "an active account", actor: "admin", email: "ada@example.com",
			hold: func(*testing.T, *testkit.Store) {}},
		{name: "a disabled invitation", actor: "admin", email: "maria@example.com",
			hold: func(t *testing.T, store *testkit.Store) {
				t.Helper()
				disable(store, addPendingMariaAs(t, store, "member"))
			}},
		{name: "an invitation beyond reach", actor: "moderator", email: "maria@example.com",
			hold: func(t *testing.T, store *testkit.Store) {
				t.Helper()
				addPendingMariaAs(t, store, "admin")
			}},
	}
	for _, tc := range cases {
		for _, sender := range []*recordingSender{{}, nil} {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				store := testkit.NewStore()
				cfg := sendingTo(nil)
				if sender != nil {
					cfg = sendingTo(sender)
				}
				routes := invitationServer(t, store, ladder(), tc.actor, cfg)
				tc.hold(t, store)

				wantDelivered(t, resendTo(t, routes.admin, tc.email))
				if sender != nil && len(sender.invites()) != 0 {
					t.Errorf("sent = %+v, want nothing sent", sender.invites())
				}
			})
		}
	}
}

func TestResendRouteRefusesAReaderWithoutAPrivilegedRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "member", sendingTo(nil))
	addPendingMariaAs(t, store, "member")

	wantRefused(t, resendTo(t, routes.admin, "maria@example.com"), http.StatusForbidden, "role_insufficient")
}

func TestResendRouteAnswersAFailedLookup(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	store.LookupErr = errLookupFailed

	wantRefused(t, resendTo(t, routes.admin, "maria@example.com"), http.StatusInternalServerError, "internal")
}

func TestActivateRouteSetsThePasswordConfirmsAndSignsIn(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	token := wantLink(t, inviteMaria(t, routes.admin, "member"))

	recorder := activate(t, routes.public, token, "a password Maria chose")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
	if identity := decodeBody[authkit.Identity](t, recorder); identity.Email != "maria@example.com" {
		t.Errorf("identity = %+v, want Maria signed in", identity)
	}
	session := doRequest(t, cookiedServer(routes.public, sessionCookie(t, recorder, testCookieName)),
		http.MethodGet, "/api/auth/session", "")
	if session.Code != http.StatusOK || decodeBody[authkit.Identity](t, session).Email != "maria@example.com" {
		t.Errorf("session = %d %s, want Maria's session", session.Code, session.Body)
	}
	if !storedMaria(t, store).Confirmed {
		t.Error("stored account is not confirmed")
	}
	login := doLogin(t, routes.public, `{"email":"maria@example.com","password":"a password Maria chose"}`)
	if login.Code != http.StatusOK {
		t.Errorf("login status = %d, want %d", login.Code, http.StatusOK)
	}
}

func TestActivateRouteRefusesASpentLink(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	token := wantLink(t, inviteMaria(t, routes.admin, "member"))
	activate(t, routes.public, token, testPassword)

	wantRefused(t, activate(t, routes.public, token, testPassword), http.StatusUnprocessableEntity, "token_invalid")
}

func TestActivateRouteRefusesAnExpiredLink(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	token := wantLink(t, inviteMaria(t, routes.admin, "member"))
	for hash, held := range store.Tokens {
		held.ExpiresAt = time.Now().UTC().Add(-time.Minute)
		store.Tokens[hash] = held
	}

	wantRefused(t, activate(t, routes.public, token, testPassword), http.StatusUnprocessableEntity, "token_invalid")
}

func TestActivateRouteRefusesAShortPasswordAndKeepsTheLink(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	token := wantLink(t, inviteMaria(t, routes.admin, "member"))

	wantRefused(t, activate(t, routes.public, token, "short"), http.StatusUnprocessableEntity, "password_too_short")
	if recorder := activate(t, routes.public, token, testPassword); recorder.Code != http.StatusOK {
		t.Errorf("activate status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
}

func TestActivateRouteCountsOnlyDeadLinksAgainstTheClient(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	limiter := newCountingLimiter(5)
	cfg := sendingTo(nil)
	cfg.Limiter = limiter
	routes := invitationServer(t, store, ladder(), "admin", cfg)
	token := wantLink(t, inviteMaria(t, routes.admin, "member"))

	activate(t, routes.public, "not-a-link", testPassword)
	activate(t, routes.public, token, "short")
	activate(t, routes.public, token, testPassword)

	if got := limiter.count(testClientAddr); got != 1 {
		t.Errorf("failures for %s = %d, want only the dead link counted", testClientAddr, got)
	}
}

func TestActivateRouteRefusesAClientOverItsBudgetAndSpendsNothing(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	cfg := sendingTo(nil)
	cfg.Limiter = newCountingLimiter(1)
	routes := invitationServer(t, store, ladder(), "admin", cfg)
	token := wantLink(t, inviteMaria(t, routes.admin, "member"))
	activate(t, routes.public, "not-a-link", testPassword)

	recorder := activate(t, routes.public, token, testPassword)

	wantRefused(t, recorder, http.StatusTooManyRequests, "link_rate_limited")
	if after := recorder.Header().Get("Retry-After"); after != "60" {
		t.Errorf("Retry-After = %q, want %q", after, "60")
	}
	if storedMaria(t, store).Confirmed {
		t.Error("the refused activation confirmed the account")
	}
}

func TestActivateRouteCountsTheClientTheKeyNames(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	limiter := newCountingLimiter(5)
	cfg := sendingTo(nil)
	cfg.Limiter = limiter
	cfg.ClientKey = func(*http.Request) string { return "client-a" }
	routes := invitationServer(t, store, ladder(), "admin", cfg)

	activate(t, routes.public, "not-a-link", testPassword)

	if got := limiter.count("client-a"); got != 1 {
		t.Errorf("failures for client-a = %d, want the dead link counted under the named key", got)
	}
}

func TestActivateRouteAnswersAFailedSessionStart(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	token := wantLink(t, inviteMaria(t, routes.admin, "member"))
	store.CreateSessionErr = errLookupFailed

	wantRefused(t, activate(t, routes.public, token, testPassword), http.StatusInternalServerError, "internal")
}

func TestInvitationRoutesRefuseAMalformedBody(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))

	for _, target := range []string{"/api/users/invite", "/api/users/invite/resend", "/api/auth/activate"} {
		wantRefused(t, doRequest(t, routes.admin, http.MethodPost, target, "{"), http.StatusBadRequest, "body_malformed")
	}
}

func TestInviteRouteAnswersAFailedReadOfTheNewAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	store.LookupErr = errLookupFailed

	wantRefused(t, inviteMaria(t, routes.admin, "member"), http.StatusInternalServerError, "internal")
}

func TestInviteRouteAnswersAFailedReadOfAHeldAddress(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	addMariaAs(t, store, "member")
	store.LookupErr = errLookupFailed

	wantRefused(t, inviteMaria(t, routes.admin, "member"), http.StatusInternalServerError, "internal")
}

func TestResendRouteAnswersAFailedLinkWrite(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	addPendingMariaAs(t, store, "member")
	store.TokenErr = errLookupFailed

	wantRefused(t, resendTo(t, routes.admin, "maria@example.com"), http.StatusInternalServerError, "internal")
}

func TestActivateRouteAnswersAFailedBudgetCheck(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	cfg := sendingTo(nil)
	cfg.Limiter = failingLimiter{checkErr: errLookupFailed}
	routes := invitationServer(t, store, ladder(), "admin", cfg)

	wantRefused(t, activate(t, routes.public, "not-a-link", testPassword), http.StatusInternalServerError, "internal")
}

func TestActivateRouteAnswersAFailedCountOfADeadLink(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	cfg := sendingTo(nil)
	cfg.Limiter = failingLimiter{recordErr: errLookupFailed}
	routes := invitationServer(t, store, ladder(), "admin", cfg)

	wantRefused(t, activate(t, routes.public, "not-a-link", testPassword), http.StatusInternalServerError, "internal")
}

func TestActivateRouteAnswersAFailedSessionRead(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(nil))
	token := wantLink(t, inviteMaria(t, routes.admin, "member"))
	store.SessionErr = errLookupFailed

	wantRefused(t, activate(t, routes.public, token, testPassword), http.StatusInternalServerError, "internal")
}

func TestNewInvitationsPanicsWithoutAnActivationURL(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Error("NewInvitations accepted a config naming no activation address")
		}
	}()
	authkit.NewInvitations(authkit.InvitationsConfig{Sender: &recordingSender{}})
}

func TestInviteRouteReadsTheNewAccountByItsNormalizedAddress(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	sender := &recordingSender{}
	routes := invitationServer(t, store, ladder(), "admin", sendingTo(sender))

	wantDelivered(t, doRequest(t, routes.admin, http.MethodPost, "/api/users/invite",
		`{"email":" Maria@Example.com ","name":"Maria Perez","role":"member"}`))

	if sent := sender.invites(); len(sent) != 1 || sent[0].to != "maria@example.com" {
		t.Errorf("sent = %+v, want one invitation to maria@example.com", sent)
	}
}

func TestActivateRouteRefusesTheLinkOfADisabledAccountAsDead(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	limiter := newCountingLimiter(5)
	cfg := sendingTo(nil)
	cfg.Limiter = limiter
	routes := invitationServer(t, store, ladder(), "admin", cfg)
	token := wantLink(t, inviteMaria(t, routes.admin, "member"))
	disable(store, storedMaria(t, store))

	wantRefused(t, activate(t, routes.public, token, testPassword), http.StatusUnprocessableEntity, "token_invalid")
	if got := limiter.count(testClientAddr); got != 1 {
		t.Errorf("failures for %s = %d, want the dead link counted", testClientAddr, got)
	}
}

func TestActivateRouteCountsOneIPv6NetworkAsOneClient(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	limiter := newCountingLimiter(5)
	cfg := sendingTo(nil)
	cfg.Limiter = limiter
	routes := invitationServer(t, store, ladder(), "admin", cfg)

	activateFrom(t, routes.public, "[2001:db8:1:2::1a]:4321", "not-a-link", testPassword)
	activateFrom(t, routes.public, "[2001:db8:1:2::2b]:4321", "not-a-link", testPassword)

	if got := limiter.count("2001:db8:1:2::"); got != 2 {
		t.Errorf("failures for 2001:db8:1:2:: = %d, want both addresses of the /64 counted together", got)
	}
}

func TestActivateRouteCountsAnAddressWithNoPortWhole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	limiter := newCountingLimiter(5)
	cfg := sendingTo(nil)
	cfg.Limiter = limiter
	routes := invitationServer(t, store, ladder(), "admin", cfg)

	activateFrom(t, routes.public, "192.0.2.9", "not-a-link", testPassword)

	if got := limiter.count("192.0.2.9"); got != 1 {
		t.Errorf("failures for 192.0.2.9 = %d, want the dead link counted under the whole address", got)
	}
}
