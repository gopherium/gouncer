// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// The lifetimes invites and resets receive when the config names none.
const (
	DefaultInviteTTL = 7 * 24 * time.Hour
	DefaultResetTTL  = time.Hour
)

// ErrAlreadyActivated reports a resend aimed at an account that already holds a password.
var ErrAlreadyActivated = errors.New("authkit: account already activated")

// InviteStore persists the accounts, tokens and sessions the invite and
// reset flows touch.
type InviteStore interface {
	gouncer.Store

	// CreateToken stores t, or returns gouncer.ErrTokenExists while a
	// live token stands for the same user and purpose.
	CreateToken(ctx context.Context, t gouncer.Token) error

	// ConsumeToken removes and returns the live token behind the hash
	// and purpose, or gouncer.ErrTokenNotFound.
	ConsumeToken(ctx context.Context, tokenHash []byte, purpose gouncer.TokenPurpose, now time.Time) (gouncer.Token, error)

	// DeleteTokensForUser removes every token the user holds for the purpose.
	DeleteTokensForUser(ctx context.Context, id uuid.UUID, purpose gouncer.TokenPurpose) error

	// ActivateAccount stores the password hash and confirms the account.
	ActivateAccount(ctx context.Context, id uuid.UUID, passwordHash string) error

	// SetUserPassword stores the password hash.
	SetUserPassword(ctx context.Context, id uuid.UUID, passwordHash string) error

	// DeleteSessionsForUser removes every session the user holds.
	DeleteSessionsForUser(ctx context.Context, id uuid.UUID) error
}

// InvitesConfig parameterizes the invite and reset flows.
type InvitesConfig struct {
	// Store persists the accounts and tokens the flows touch.
	Store InviteStore
	// InviteTTL is how long an invite token lives. Zero applies DefaultInviteTTL.
	InviteTTL time.Duration
	// ResetTTL is how long a reset token lives. Zero applies DefaultResetTTL.
	ResetTTL time.Duration
}

// Invites offers the invite and reset flows over a store. The token
// secrets it returns are for the caller to deliver, never persisted.
type Invites struct {
	store     InviteStore
	inviteTTL time.Duration
	resetTTL  time.Duration
}

// NewInvites returns Invites over cfg's store with its lifetimes.
func NewInvites(cfg InvitesConfig) *Invites {
	inviteTTL := cfg.InviteTTL
	if inviteTTL == 0 {
		inviteTTL = DefaultInviteTTL
	}
	resetTTL := cfg.ResetTTL
	if resetTTL == 0 {
		resetTTL = DefaultResetTTL
	}
	return &Invites{store: cfg.Store, inviteTTL: inviteTTL, resetTTL: resetTTL}
}

// Invite creates an unconfirmed account under the role and answers the
// token whose secret activates it.
func (i *Invites) Invite(ctx context.Context, email, name, role string) (gouncer.Token, error) {
	u, err := gouncer.NewInvitedUser(email, name)
	if err != nil {
		return gouncer.Token{}, err
	}
	u.Role = role
	if err := i.store.CreateUser(ctx, u); err != nil {
		return gouncer.Token{}, err
	}
	return i.issue(ctx, u.ID, gouncer.PurposeInvite, i.inviteTTL)
}

// ResendInvite replaces the pending token of an unactivated account,
// answering the fresh one.
func (i *Invites) ResendInvite(ctx context.Context, email string) (gouncer.Token, error) {
	u, err := i.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return gouncer.Token{}, err
	}
	if u.Confirmed || u.PasswordHash != "" {
		return gouncer.Token{}, ErrAlreadyActivated
	}
	if err := i.store.DeleteTokensForUser(ctx, u.ID, gouncer.PurposeInvite); err != nil {
		return gouncer.Token{}, err
	}
	return i.issue(ctx, u.ID, gouncer.PurposeInvite, i.inviteTTL)
}

// RedeemInvite consumes an invite token, storing the password,
// confirming the address, and answering the activated account's id.
func (i *Invites) RedeemInvite(ctx context.Context, token, password string) (uuid.UUID, error) {
	hash, err := hashOf(password)
	if err != nil {
		return uuid.Nil, err
	}
	t, err := i.store.ConsumeToken(ctx, gouncer.HashToken(token), gouncer.PurposeInvite, time.Now().UTC())
	if err != nil {
		return uuid.Nil, err
	}
	if err := i.store.ActivateAccount(ctx, t.UserID, hash); err != nil {
		return uuid.Nil, err
	}
	return t.UserID, nil
}

// RequestReset issues a reset token for a confirmed enabled account,
// answering gouncer.ErrUserNotFound for every other address.
func (i *Invites) RequestReset(ctx context.Context, email string) (gouncer.Token, error) {
	u, err := i.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return gouncer.Token{}, err
	}
	if u.Disabled || !u.Confirmed {
		return gouncer.Token{}, gouncer.ErrUserNotFound
	}
	return i.issue(ctx, u.ID, gouncer.PurposeReset, i.resetTTL)
}

// RedeemReset consumes a reset token, ending every session the account
// holds before replacing the password, answering the account's id. The
// sessions go first so a failure never leaves one live beside a
// password its holder no longer knows.
func (i *Invites) RedeemReset(ctx context.Context, token, password string) (uuid.UUID, error) {
	hash, err := hashOf(password)
	if err != nil {
		return uuid.Nil, err
	}
	t, err := i.store.ConsumeToken(ctx, gouncer.HashToken(token), gouncer.PurposeReset, time.Now().UTC())
	if err != nil {
		return uuid.Nil, err
	}
	if err := i.store.DeleteSessionsForUser(ctx, t.UserID); err != nil {
		return uuid.Nil, err
	}
	if err := i.store.SetUserPassword(ctx, t.UserID, hash); err != nil {
		return uuid.Nil, err
	}
	return t.UserID, nil
}

// issue creates and stores a token for the user, answering it with its
// secret. The store is handed the hash alone, so no implementation of
// InviteStore is ever in a position to persist the secret itself.
func (i *Invites) issue(
	ctx context.Context,
	userID uuid.UUID,
	purpose gouncer.TokenPurpose,
	ttl time.Duration,
) (gouncer.Token, error) {
	t, err := gouncer.NewToken(userID, purpose, ttl)
	if err != nil {
		return gouncer.Token{}, err
	}
	stored := t
	stored.Token = ""
	if err := i.store.CreateToken(ctx, stored); err != nil {
		return gouncer.Token{}, err
	}
	return t, nil
}

// hashOf validates the password under the gouncer bounds and answers its hash.
func hashOf(password string) (string, error) {
	var scratch gouncer.User
	if err := scratch.SetPassword(password); err != nil {
		return "", err
	}
	return scratch.PasswordHash, nil
}
