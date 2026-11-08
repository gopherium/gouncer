// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/cucumber/godog"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/ratelimit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

var (
	_ authkit.InviteSender = (*recordingSender)(nil)
	_ authkit.LinkLimiter  = (*ratelimit.Limiter)(nil)
)

const activationURL = "https://example.com/admin/activate"

const signedInPassword = "correct horse battery"

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
	return slices.Clone(s.sent)
}

// roleTable names the capabilities each role carries.
type roleTable map[string][]string

// Known reports whether the table holds the role.
func (t roleTable) Known(role string) bool {
	_, ok := t[role]
	return ok
}

// Outranks reports whether the caller carries every capability the target carries.
func (t roleTable) Outranks(caller, target string) bool {
	carried, ok := t[target]
	if !ok {
		return false
	}
	for _, capability := range carried {
		if !slices.Contains(t[caller], capability) {
			return false
		}
	}
	return true
}

// roles is the table every scenario runs on, where admin outranks moderator and moderator outranks member.
var roles = roleTable{
	"admin":     {"settings.manage", "users.manage"},
	"moderator": {"users.manage"},
	"member":    nil,
}

// roleOf returns the role a reader named in a scenario holds.
var roleOf = map[string]string{"administrator": "admin", "moderator": "moderator"}

// deliveryBody is the delivery report an invite or a resend answers.
type deliveryBody struct {
	Delivered      *bool   `json:"delivered"`
	ActivationLink *string `json:"activation_link"`
}

// errorBody is the error shape a client reads.
type errorBody struct {
	Code string `json:"code"`
}

// invitationsWorld is what one invitation scenario builds and finds.
type invitationsWorld struct {
	store      *testkit.Store
	sender     *recordingSender
	limiter    authkit.LinkLimiter
	admin      http.Handler
	public     http.Handler
	answer     *httptest.ResponseRecorder
	activation *httptest.ResponseRecorder
	tokens     []string
	held       gouncer.User
}

// sendsMail makes the app deliver each link through a sender that records it.
func (w *invitationsWorld) sendsMail() {
	w.sender = &recordingSender{}
}

// sendsNoMail leaves the app without a sender, so each link comes back in the answer.
func (w *invitationsWorld) sendsNoMail() {
	w.sender = nil
}

// relayRefuses makes the sender fail every mail.
func (w *invitationsWorld) relayRefuses() {
	w.sender = &recordingSender{err: errRelayDown}
}

// limitsDeadLinks caps the dead links each client may try.
func (w *invitationsWorld) limitsDeadLinks(limit int) {
	w.limiter = ratelimit.NewLimiter(ratelimit.Config{Limit: limit})
}

// signsIn mounts the routes and signs the reader in under the role the scenario names.
func (w *invitationsWorld) signsIn(reader string) error {
	ada, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", signedInPassword)
	if err != nil {
		return err
	}
	ada.Role = roleOf[reader]
	if err := w.store.CreateUser(context.Background(), ada); err != nil {
		return err
	}
	w.mount()
	login := postJSON(w.public, "/api/auth/login", map[string]string{"email": ada.Email, "password": signedInPassword})
	if login.Code != http.StatusOK {
		return fmt.Errorf("login status = %d, body %s", login.Code, login.Body)
	}
	cookies := login.Result().Cookies()
	if len(cookies) == 0 {
		return errors.New("login set no cookie")
	}
	mux := w.public
	w.admin = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		r.AddCookie(cookies[0])
		mux.ServeHTTP(rw, r)
	})
	return nil
}

// mount builds the login, session and invitation routes over the scenario's store.
func (w *invitationsWorld) mount() {
	privileged := gouncer.Roles{"admin", "moderator"}
	h := authkit.New(authkit.Config{Store: w.store, Privileged: privileged})
	cfg := authkit.InvitationsConfig{
		Invites:       authkit.NewInvites(authkit.InvitesConfig{Store: w.store, Roles: roles}),
		Sessions:      h,
		Admin:         authkit.NewAdmin(authkit.AdminConfig{Store: w.store, Privileged: privileged, Roles: roles}),
		ActivationURL: activationURL,
		Limiter:       w.limiter,
	}
	if w.sender != nil {
		cfg.Sender = w.sender
	}
	invitations := authkit.NewInvitations(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.Handle("GET /api/auth/session", h.RequireSession(http.HandlerFunc(h.Session)))
	mux.Handle("POST /api/users/invite", h.RequireSession(http.HandlerFunc(invitations.Invite)))
	mux.Handle("POST /api/users/invite/resend", h.RequireSession(http.HandlerFunc(invitations.Resend)))
	mux.HandleFunc("POST /api/auth/activate", invitations.Activate)
	w.public = mux
}

// postJSON sends the fields as a JSON object to the route and records the response.
func postJSON(handler http.Handler, target string, fields map[string]string) *httptest.ResponseRecorder {
	encoded, _ := json.Marshal(fields)
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(string(encoded)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// holdsActive stores an active account for the address and remembers it as it stood.
func (w *invitationsWorld) holdsActive(email string) error {
	held, err := gouncer.NewUser(email, "Maria Perez", signedInPassword)
	if err != nil {
		return err
	}
	held.Role = "member"
	w.held = held
	return w.store.CreateUser(context.Background(), held)
}

// invites posts an invitation and notes the link it gave out.
func (w *invitationsWorld) invites(email, name, role string) error {
	w.answer = postJSON(w.admin, "/api/users/invite", map[string]string{"email": email, "name": name, "role": role})
	return w.noteLink()
}

// wasInvited invites the address and requires the invitation to go through.
func (w *invitationsWorld) wasInvited(email, name, role string) error {
	if err := w.invites(email, name, role); err != nil {
		return err
	}
	if w.answer.Code != http.StatusOK {
		return fmt.Errorf("invite status = %d, body %s", w.answer.Code, w.answer.Body)
	}
	return nil
}

// resends posts a resend for the address and notes the link it gave out.
func (w *invitationsWorld) resends(email string) error {
	w.answer = postJSON(w.admin, "/api/users/invite/resend", map[string]string{"email": email})
	return w.noteLink()
}

// noteLink records the token of the link the last answer or the sender's last mail carries.
func (w *invitationsWorld) noteLink() error {
	link := ""
	if body, ok := decode[deliveryBody](w.answer); ok && body.ActivationLink != nil {
		link = *body.ActivationLink
	}
	if sent := w.sentMails(); link == "" && len(sent) > 0 && w.answer.Code == http.StatusOK {
		link = sent[len(sent)-1].link
	}
	if link == "" {
		return nil
	}
	parsed, err := url.Parse(link)
	if err != nil {
		return err
	}
	token := parsed.Query().Get("token")
	if slices.Contains(w.tokens, token) {
		return nil
	}
	w.tokens = append(w.tokens, token)
	return nil
}

// sentMails returns every mail the app sent, none when it sends no mail.
func (w *invitationsWorld) sentMails() []sentInvite {
	if w.sender == nil {
		return nil
	}
	return w.sender.invites()
}

// activates opens the link with the password from the default client address.
func (w *invitationsWorld) activates(token, password string) *httptest.ResponseRecorder {
	return postJSON(w.public, "/api/auth/activate", map[string]string{"token": token, "password": password})
}

// opensLatest opens the latest link with the password.
func (w *invitationsWorld) opensLatest(password string) error {
	if len(w.tokens) == 0 {
		return errors.New("no link was given out")
	}
	w.activation = w.activates(w.tokens[len(w.tokens)-1], password)
	return nil
}

// openedLatest opens the latest link and requires it to activate.
func (w *invitationsWorld) openedLatest(password string) error {
	if err := w.opensLatest(password); err != nil {
		return err
	}
	if w.activation.Code != http.StatusOK {
		return fmt.Errorf("activate status = %d, body %s", w.activation.Code, w.activation.Body)
	}
	return nil
}

// triedDeadLinks opens count links no invitation gave out.
func (w *invitationsWorld) triedDeadLinks(count int) error {
	for i := range count {
		recorder := w.activates(fmt.Sprintf("dead-link-%d", i), signedInPassword)
		if recorder.Code != http.StatusUnprocessableEntity {
			return fmt.Errorf("dead link %d status = %d, body %s", i, recorder.Code, recorder.Body)
		}
	}
	return nil
}

// decode unmarshals the recorded body, reporting false when it is not that shape.
func decode[T any](recorder *httptest.ResponseRecorder) (T, bool) {
	var v T
	err := json.Unmarshal(recorder.Body.Bytes(), &v)
	return v, err == nil
}

// refusedWith fails unless the response carries the status and the code.
func refusedWith(recorder *httptest.ResponseRecorder, status int, code string) error {
	body, _ := decode[errorBody](recorder)
	if recorder.Code != status || body.Code != code {
		return fmt.Errorf("answer = %d %s, want %d with code %q", recorder.Code, recorder.Body, status, code)
	}
	return nil
}

// saysDelivered fails unless the answer reports the link delivered and carries none.
func (w *invitationsWorld) saysDelivered() error {
	body, ok := decode[deliveryBody](w.answer)
	if w.answer.Code != http.StatusOK || !ok || body.Delivered == nil || !*body.Delivered || body.ActivationLink != nil {
		return fmt.Errorf("answer = %d %s, want delivered and no link", w.answer.Code, w.answer.Body)
	}
	return nil
}

// carriesLink fails unless the answer hands back a link to the activation address.
func (w *invitationsWorld) carriesLink() error {
	body, ok := decode[deliveryBody](w.answer)
	if w.answer.Code != http.StatusOK || !ok || body.Delivered == nil || *body.Delivered ||
		body.ActivationLink == nil || !strings.HasPrefix(*body.ActivationLink, activationURL+"?token=") {
		return fmt.Errorf("answer = %d %s, want an undelivered link to %s", w.answer.Code, w.answer.Body, activationURL)
	}
	return nil
}

// received fails unless the address got count mails, each greeting the name.
func (w *invitationsWorld) received(email string, count int, name string) error {
	got := 0
	for _, sent := range w.sentMails() {
		if sent.to == email && sent.name == name {
			got++
		}
	}
	if got != count {
		return fmt.Errorf("mails to %s greeting %s = %d, want %d (all sent %+v)", email, name, got, count, w.sentMails())
	}
	return nil
}

// noMail fails when the app sent any mail.
func (w *invitationsWorld) noMail() error {
	if sent := w.sentMails(); len(sent) != 0 {
		return fmt.Errorf("sent = %+v, want no mail", sent)
	}
	return nil
}

// stored returns the account the address holds.
func (w *invitationsWorld) stored(email string) (gouncer.User, error) {
	return w.store.UserByEmail(context.Background(), email)
}

// waitsForActivation fails unless the address holds an unconfirmed account with no password under the role.
func (w *invitationsWorld) waitsForActivation(email, role string) error {
	account, err := w.stored(email)
	if err != nil {
		return err
	}
	if account.Confirmed || account.PasswordHash != "" || account.Role != role {
		return fmt.Errorf("stored = %+v, want an unconfirmed %s with no password", account, role)
	}
	return nil
}

// roleRefused fails unless the invitation was refused for a role the reader may not give.
func (w *invitationsWorld) roleRefused() error {
	return refusedWith(w.answer, http.StatusForbidden, "role_not_grantable")
}

// noAccount fails when the address holds an account.
func (w *invitationsWorld) noAccount(email string) error {
	if account, err := w.stored(email); !errors.Is(err, gouncer.ErrUserNotFound) {
		return fmt.Errorf("stored = %+v (error %v), want no account", account, err)
	}
	return nil
}

// unchanged fails unless the address holds the account exactly as it stood.
func (w *invitationsWorld) unchanged(email string) error {
	account, err := w.stored(email)
	if err != nil {
		return err
	}
	if account.Name != w.held.Name || account.Role != w.held.Role || account.Confirmed != w.held.Confirmed ||
		account.Disabled != w.held.Disabled || account.PasswordHash != w.held.PasswordHash {
		return fmt.Errorf("stored = %+v, want it unchanged from %+v", account, w.held)
	}
	return nil
}

// earlierDead fails unless the link given out before the latest one is refused.
func (w *invitationsWorld) earlierDead() error {
	if len(w.tokens) < 2 {
		return fmt.Errorf("links given out = %d, want at least 2", len(w.tokens))
	}
	return refusedWith(w.activates(w.tokens[len(w.tokens)-2], signedInPassword),
		http.StatusUnprocessableEntity, "token_invalid")
}

// latestActivates fails unless the latest link activates.
func (w *invitationsWorld) latestActivates() error {
	return w.openedLatest(signedInPassword)
}

// keepsName fails unless the address's account still holds the name.
func (w *invitationsWorld) keepsName(email, name string) error {
	account, err := w.stored(email)
	if err != nil {
		return err
	}
	if account.Name != name {
		return fmt.Errorf("stored name = %q, want %q", account.Name, name)
	}
	return nil
}

// undelivered fails unless the invitation was reported undelivered.
func (w *invitationsWorld) undelivered() error {
	return refusedWith(w.answer, http.StatusBadGateway, "delivery_failed")
}

// signedInAs fails unless the activation started a session for the address.
func (w *invitationsWorld) signedInAs(email string) error {
	identity, _ := decode[authkit.Identity](w.activation)
	if w.activation.Code != http.StatusOK || identity.Email != email {
		return fmt.Errorf("activation = %d %s, want %s signed in", w.activation.Code, w.activation.Body, email)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	for _, cookie := range w.activation.Result().Cookies() {
		request.AddCookie(cookie)
	}
	session := httptest.NewRecorder()
	w.public.ServeHTTP(session, request)
	if found, _ := decode[authkit.Identity](session); session.Code != http.StatusOK || found.Email != email {
		return fmt.Errorf("session = %d %s, want %s", session.Code, session.Body, email)
	}
	return nil
}

// logsIn fails unless the address logs in with the password.
func (w *invitationsWorld) logsIn(email, password string) error {
	login := postJSON(w.public, "/api/auth/login", map[string]string{"email": email, "password": password})
	if login.Code != http.StatusOK {
		return fmt.Errorf("login status = %d, body %s", login.Code, login.Body)
	}
	return nil
}

// linkRefused fails unless the activation was refused as a dead link.
func (w *invitationsWorld) linkRefused() error {
	return refusedWith(w.activation, http.StatusUnprocessableEntity, "token_invalid")
}

// passwordTooShort fails unless the activation was refused for a password under the minimum.
func (w *invitationsWorld) passwordTooShort() error {
	return refusedWith(w.activation, http.StatusUnprocessableEntity, "password_too_short")
}

// tooManyAttempts fails unless the activation was refused for the client's spent budget.
func (w *invitationsWorld) tooManyAttempts() error {
	return refusedWith(w.activation, http.StatusTooManyRequests, "link_rate_limited")
}

// initializeInvitations binds the invitation steps to a fresh world for each scenario.
func initializeInvitations(sc *godog.ScenarioContext) {
	w := &invitationsWorld{store: testkit.NewStore()}
	sc.Given(`^the app sends mail$`, w.sendsMail)
	sc.Given(`^the app sends no mail$`, w.sendsNoMail)
	sc.Given(`^the mail relay refuses every mail$`, w.relayRefuses)
	sc.Given(`^each client may try (\d+) dead links?$`, w.limitsDeadLinks)
	sc.Given(`^an? (administrator|moderator) is signed in$`, w.signsIn)
	sc.Given(`^"([^"]*)" already holds an active account$`, w.holdsActive)
	sc.Given(`^"([^"]*)" was invited as "([^"]*)" with the role "([^"]*)"$`, w.wasInvited)
	sc.Given(`^the person opened the latest link and chose the password "([^"]*)"$`, w.openedLatest)
	sc.Given(`^a client tried (\d+) dead links?$`, w.triedDeadLinks)
	sc.When(`^"([^"]*)" is invited as "([^"]*)" with the role "([^"]*)"$`, w.invites)
	sc.When(`^the invitation to "([^"]*)" is sent again$`, w.resends)
	sc.When(`^the (?:person|same client) opens the latest link and chooses the password "([^"]*)"$`, w.opensLatest)
	sc.Then(`^the answer says the link was delivered$`, w.saysDelivered)
	sc.Then(`^the answer carries the activation link$`, w.carriesLink)
	sc.Then(`^"([^"]*)" received (\d+) mails? greeting "([^"]*)"$`, w.received)
	sc.Then(`^no mail was sent$`, w.noMail)
	sc.Then(`^the account of "([^"]*)" waits for activation with the role "([^"]*)"$`, w.waitsForActivation)
	sc.Then(`^the invitation is refused because the role may not be given$`, w.roleRefused)
	sc.Then(`^no account holds "([^"]*)"$`, w.noAccount)
	sc.Then(`^the account of "([^"]*)" is unchanged$`, w.unchanged)
	sc.Then(`^the earlier link no longer activates$`, w.earlierDead)
	sc.Then(`^the latest link activates$`, w.latestActivates)
	sc.Then(`^the account of "([^"]*)" keeps the name "([^"]*)"$`, w.keepsName)
	sc.Then(`^the invitation is reported undelivered$`, w.undelivered)
	sc.Then(`^the person is signed in as "([^"]*)"$`, w.signedInAs)
	sc.Then(`^"([^"]*)" can log in with the password "([^"]*)"$`, w.logsIn)
	sc.Then(`^the link is refused as no longer valid$`, w.linkRefused)
	sc.Then(`^the password is refused as too short$`, w.passwordTooShort)
	sc.Then(`^the activation is refused as too many attempts$`, w.tooManyAttempts)
}
