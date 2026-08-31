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

	// CreateToken stores t for an enabled account, or returns
	// gouncer.ErrTokenExists while a live token stands for the same user
	// and purpose and gouncer.ErrUserNotFound for a disabled one.
	CreateToken(ctx context.Context, t gouncer.Token) error

	// ReplaceToken stores t for an enabled account in place of every
	// token it holds for the same purpose, as one change that either
	// lands whole or not at all, answering gouncer.ErrUserNotFound for a
	// disabled account.
	ReplaceToken(ctx context.Context, t gouncer.Token) error

	// ActivateByToken spends the invite token, storing the password hash
	// and confirming the account it names, as one change that either
	// lands whole or not at all. It answers the account's id,
	// gouncer.ErrTokenNotFound for a spent or expired token, and
	// gouncer.ErrUserNotFound for a disabled or confirmed account.
	ActivateByToken(ctx context.Context, tokenHash []byte, now time.Time, passwordHash string) (uuid.UUID, error)

	// ResetByToken spends the reset token, storing the password hash and
	// ending every session the account it names holds, as one change
	// that either lands whole or not at all. It answers the account's
	// id, gouncer.ErrTokenNotFound for a spent or expired token, and
	// gouncer.ErrUserNotFound for a disabled account.
	ResetByToken(ctx context.Context, tokenHash []byte, now time.Time, passwordHash string) (uuid.UUID, error)
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
	return i.issue(ctx, u.ID, gouncer.PurposeInvite, i.inviteTTL, i.store.CreateToken)
}

// ResendInvite replaces the pending token of an unactivated enabled
// account, answering the fresh one and gouncer.ErrUserNotFound for
// every other address.
func (i *Invites) ResendInvite(ctx context.Context, email string) (gouncer.Token, error) {
	u, err := i.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return gouncer.Token{}, err
	}
	if u.Disabled {
		return gouncer.Token{}, gouncer.ErrUserNotFound
	}
	if u.Confirmed || u.PasswordHash != "" {
		return gouncer.Token{}, ErrAlreadyActivated
	}
	return i.issue(ctx, u.ID, gouncer.PurposeInvite, i.inviteTTL, i.store.ReplaceToken)
}

// RedeemInvite spends an invite token, storing the password, confirming
// the address, and answering the activated account's id as one store
// change.
func (i *Invites) RedeemInvite(ctx context.Context, token, password string) (uuid.UUID, error) {
	hash, err := hashOf(password)
	if err != nil {
		return uuid.Nil, err
	}
	return i.store.ActivateByToken(ctx, gouncer.HashToken(token), time.Now().UTC(), hash)
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
	return i.issue(ctx, u.ID, gouncer.PurposeReset, i.resetTTL, i.store.CreateToken)
}

// ResendReset replaces the standing reset token of a confirmed enabled
// account, answering the fresh one and gouncer.ErrUserNotFound for every
// other address.
func (i *Invites) ResendReset(ctx context.Context, email string) (gouncer.Token, error) {
	u, err := i.store.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return gouncer.Token{}, err
	}
	if u.Disabled || !u.Confirmed {
		return gouncer.Token{}, gouncer.ErrUserNotFound
	}
	return i.issue(ctx, u.ID, gouncer.PurposeReset, i.resetTTL, i.store.ReplaceToken)
}

// RedeemReset spends a reset token, replacing the password and ending
// every session the account holds as one store change, answering the
// account's id.
func (i *Invites) RedeemReset(ctx context.Context, token, password string) (uuid.UUID, error) {
	hash, err := hashOf(password)
	if err != nil {
		return uuid.Nil, err
	}
	return i.store.ResetByToken(ctx, gouncer.HashToken(token), time.Now().UTC(), hash)
}

// issue mints a token for the user and stores it with write, answering
// it with its secret. The store is handed the hash alone, so no
// implementation of InviteStore is ever in a position to persist the
// secret itself.
func (i *Invites) issue(
	ctx context.Context,
	userID uuid.UUID,
	purpose gouncer.TokenPurpose,
	ttl time.Duration,
	write func(context.Context, gouncer.Token) error,
) (gouncer.Token, error) {
	t, err := gouncer.NewToken(userID, purpose, ttl)
	if err != nil {
		return gouncer.Token{}, err
	}
	stored := t
	stored.Token = ""
	if err := write(ctx, stored); err != nil {
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
