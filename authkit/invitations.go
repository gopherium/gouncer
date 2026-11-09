// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// errLeftReach reports an account that left the actor's reach while its link was renewed.
var errLeftReach = errors.New("authkit: account left reach")

// InviteSender delivers the link that activates an invited account.
type InviteSender interface {
	// SendInvite delivers link to the address to, greeting name.
	SendInvite(ctx context.Context, to, name, link string) error
}

// LinkLimiter caps the dead activation links one client may spend.
type LinkLimiter interface {
	// Check reports whether key may spend a link, and the wait when it may not.
	Check(key string) (bool, time.Duration, error)
	// RecordFailure counts one dead link against key.
	RecordFailure(key string) error
}

// InvitationsConfig parameterizes the invite, resend and activate routes.
type InvitationsConfig struct {
	// Invites issues, replaces and spends the activation links.
	Invites *Invites
	// Sessions starts the session an activated account signs in with.
	Sessions *Handlers
	// Admin judges who may invite or resend and which roles they may give.
	Admin *AdminHandlers
	// Sender delivers each link. Nil answers the link to the caller instead.
	Sender InviteSender
	// ActivationURL is the address a link opens, which gains the token as its token query value. It is required.
	ActivationURL string
	// Limiter caps the dead links each client spends. Nil caps none.
	Limiter LinkLimiter
	// ClientKey names the client a request counts against. Nil keys the remote address, IPv6 by its /64 network.
	ClientKey func(*http.Request) string
}

// Invitations serves the invite and resend routes behind RequireSession and the public activate route.
type Invitations struct {
	invites       *Invites
	sessions      *Handlers
	admin         *AdminHandlers
	sender        InviteSender
	activationURL url.URL
	limiter       LinkLimiter
	clientKey     func(*http.Request) string
}

// NewInvitations returns the invitation routes cfg describes, panicking on a missing or unparsable ActivationURL.
func NewInvitations(cfg InvitationsConfig) *Invitations {
	if cfg.ActivationURL == "" {
		panic("authkit: InvitationsConfig.ActivationURL is required")
	}
	activationURL, err := url.Parse(cfg.ActivationURL)
	if err != nil {
		panic("authkit: InvitationsConfig.ActivationURL does not parse: " + err.Error())
	}
	clientKey := cfg.ClientKey
	if clientKey == nil {
		clientKey = remoteHost
	}
	return &Invitations{
		invites:       cfg.Invites,
		sessions:      cfg.Sessions,
		admin:         cfg.Admin,
		sender:        cfg.Sender,
		activationURL: *activationURL,
		limiter:       cfg.Limiter,
		clientKey:     clientKey,
	}
}

// delivery is the report an invite or a resend answers.
type delivery struct {
	Delivered      bool   `json:"delivered"`
	ActivationLink string `json:"activation_link,omitempty"`
}

// delivered is the report of a link the sender took, and of every address with no invitation to renew.
var delivered = delivery{Delivered: true}

// Invite creates an invited account under the role and delivers its link, answering a held address as Resend does.
func (v *Invitations) Invite(w http.ResponseWriter, r *http.Request) {
	if v.admin.refuseUnprivileged(w, r) {
		return
	}
	type request struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}
	req, err := Decode[request](w, r)
	if err != nil {
		respondDecodeError(w, err, "malformed json")
		return
	}
	if v.admin.refuseHandout(w, r, req.Role) {
		return
	}
	token, err := v.invites.Invite(r.Context(), req.Email, req.Name, req.Role)
	if errors.Is(err, gouncer.ErrEmailTaken) {
		v.resend(w, r, req.Email)
		return
	}
	if err != nil {
		respondAuthError(w, err)
		return
	}
	invited, err := v.admin.store.UserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(req.Email)))
	if err != nil {
		respondAuthError(w, err)
		return
	}
	v.deliver(r.Context(), w, invited, token)
}

// Resend replaces a pending account's link and delivers the new one, answering delivered for any other address.
func (v *Invitations) Resend(w http.ResponseWriter, r *http.Request) {
	if v.admin.refuseUnprivileged(w, r) {
		return
	}
	type request struct {
		Email string `json:"email"`
	}
	req, err := Decode[request](w, r)
	if err != nil {
		respondDecodeError(w, err, "malformed json")
		return
	}
	v.resend(w, r, req.Email)
}

// resend delivers a fresh link for the pending account behind email, answering delivered for any other address.
func (v *Invitations) resend(w http.ResponseWriter, r *http.Request, email string) {
	held, token, renewed, err := v.renew(r, email)
	if err != nil {
		respondAuthError(w, err)
		return
	}
	if !renewed {
		Respond(w, http.StatusOK, delivered)
		return
	}
	v.deliver(r.Context(), w, held, token)
}

// renew replaces the link of the pending account behind email, reporting false when the actor has none to renew.
func (v *Invitations) renew(r *http.Request, email string) (gouncer.User, gouncer.Token, bool, error) {
	held, open, err := v.reachable(r, email)
	if err != nil || !open {
		return gouncer.User{}, gouncer.Token{}, false, err
	}
	token, err := v.invites.stageInvite(r.Context(), held)
	if err == nil {
		err = v.settle(r, held, token)
	}
	if errors.Is(err, gouncer.ErrUserNotFound) || errors.Is(err, ErrAlreadyActivated) || errors.Is(err, errLeftReach) {
		return gouncer.User{}, gouncer.Token{}, false, nil
	}
	if err != nil {
		return gouncer.User{}, gouncer.Token{}, false, err
	}
	return held, token, true, nil
}

// settle keeps the staged link alone once the account still stands within the actor's reach.
func (v *Invitations) settle(r *http.Request, held gouncer.User, staged gouncer.Token) error {
	kept, err := v.stillReachable(r, held)
	if err != nil {
		return err
	}
	if !kept {
		return errLeftReach
	}
	return v.invites.settleInvite(r.Context(), staged)
}

// stillReachable reports whether the account still holds the role it was judged by and the actor may still edit it.
func (v *Invitations) stillReachable(r *http.Request, judged gouncer.User) (bool, error) {
	now, open, err := v.reachable(r, judged.Email)
	return open && now.Role == judged.Role, err
}

// reachable returns the account behind email and whether the actor may edit it.
func (v *Invitations) reachable(r *http.Request, email string) (gouncer.User, bool, error) {
	held, err := v.admin.store.UserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(email)))
	if errors.Is(err, gouncer.ErrUserNotFound) {
		return gouncer.User{}, false, nil
	}
	if err != nil {
		return gouncer.User{}, false, err
	}
	reader := IdentityFromContext(r.Context()).Role
	return held, v.admin.editable(reader, held.Role, declared(v.admin.roles, held.Role)), nil
}

// deliver hands the account's link to the sender, or answers it to the caller when no sender is set.
func (v *Invitations) deliver(ctx context.Context, w http.ResponseWriter, to gouncer.User, token gouncer.Token) {
	link := v.link(token.Token)
	if v.sender == nil {
		Respond(w, http.StatusOK, delivery{ActivationLink: link})
		return
	}
	if err := v.sender.SendInvite(ctx, to.Email, to.Name, link); err != nil {
		RespondError(w, http.StatusBadGateway, ErrorResponse{
			Message: "the invitation could not be delivered", Code: "delivery_failed",
		})
		return
	}
	Respond(w, http.StatusOK, delivered)
}

// link returns the activation address with the token as the only token value of its query.
func (v *Invitations) link(token string) string {
	address := v.activationURL
	query := address.Query()
	query.Set("token", token)
	address.RawQuery = query.Encode()
	return address.String()
}

// Activate spends an activation link, setting the password, confirming the account and starting its session.
func (v *Invitations) Activate(w http.ResponseWriter, r *http.Request) {
	key := v.clientKey(r)
	if v.refuseOverBudget(w, key) {
		return
	}
	type request struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	req, err := Decode[request](w, r)
	if err != nil {
		respondDecodeError(w, err, "malformed json")
		return
	}
	id, err := v.invites.RedeemInvite(r.Context(), req.Token, req.Password)
	if errors.Is(err, gouncer.ErrTokenNotFound) || errors.Is(err, gouncer.ErrUserNotFound) {
		v.refuseDeadLink(w, key)
		return
	}
	if err != nil {
		respondAuthError(w, err)
		return
	}
	v.signIn(r.Context(), w, id)
}

// refuseOverBudget reports whether it refused a client that already spent its dead links.
func (v *Invitations) refuseOverBudget(w http.ResponseWriter, key string) bool {
	if v.limiter == nil {
		return false
	}
	allowed, retryAfter, err := v.limiter.Check(key)
	if err != nil {
		respondAuthError(w, err)
		return true
	}
	if allowed {
		return false
	}
	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
	RespondError(w, http.StatusTooManyRequests, ErrorResponse{
		Message: "too many attempts, try again later", Code: "link_rate_limited",
	})
	return true
}

// refuseDeadLink answers a spent, expired or unknown link, counting it against key.
func (v *Invitations) refuseDeadLink(w http.ResponseWriter, key string) {
	if v.limiter != nil {
		if err := v.limiter.RecordFailure(key); err != nil {
			respondAuthError(w, err)
			return
		}
	}
	RespondError(w, http.StatusUnprocessableEntity, ErrorResponse{
		Message: "this link is no longer valid", Code: "token_invalid",
	})
}

// signIn starts the activated account's session and answers its identity.
func (v *Invitations) signIn(ctx context.Context, w http.ResponseWriter, id uuid.UUID) {
	cookie, err := v.sessions.StartSession(ctx, id)
	if err != nil {
		respondAuthError(w, err)
		return
	}
	identity, err := v.sessions.SessionIdentity(ctx, cookie.Value)
	if err != nil {
		respondAuthError(w, err)
		return
	}
	http.SetCookie(w, cookie)
	Respond(w, http.StatusOK, identity)
}

// remoteHost returns the remote address's IP, an IPv4 one whole and an IPv6 one cut to its /64 on its link.
func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	ip = ip.Unmap()
	if ip.Is4() {
		return ip.String()
	}
	network, _ := ip.Prefix(64)
	return network.Addr().WithZone(ip.Zone()).String()
}
