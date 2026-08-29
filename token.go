// SPDX-License-Identifier: Apache-2.0

package gouncer

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrEmptyPurpose reports a token requested without a purpose.
var ErrEmptyPurpose = errors.New("gouncer: empty token purpose")

// ErrTokenLifetime reports a token requested with a lifetime that is not positive.
var ErrTokenLifetime = errors.New("gouncer: token lifetime not positive")

// ErrTokenNotFound reports that no usable token exists for a hash and purpose:
// it is unknown, expired, or already consumed.
var ErrTokenNotFound = errors.New("gouncer: token not found")

// ErrTokenExists reports that the user already holds a live token for the purpose.
var ErrTokenExists = errors.New("gouncer: token already exists")

// TokenPurpose names what redeeming a token does.
type TokenPurpose string

// The purposes a token is issued for.
const (
	PurposeInvite  TokenPurpose = "invite"
	PurposeReset   TokenPurpose = "reset"
	PurposeConfirm TokenPurpose = "confirm"
)

const tokenSecretBytes = 32

// Token is a single-use secret sent to a user. Build one with [NewToken].
// Token is handed out once, only TokenHash is persisted.
type Token struct {
	Token     string
	TokenHash []byte
	UserID    uuid.UUID
	Purpose   TokenPurpose
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewToken issues a token for the user with a fresh random secret living for ttl.
func NewToken(userID uuid.UUID, purpose TokenPurpose, ttl time.Duration) (Token, error) {
	if purpose == "" {
		return Token{}, ErrEmptyPurpose
	}
	if ttl <= 0 {
		return Token{}, ErrTokenLifetime
	}
	raw := make([]byte, tokenSecretBytes)
	if _, err := randRead(raw); err != nil {
		return Token{}, fmt.Errorf("gouncer: generate token secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	return Token{
		Token:     secret,
		TokenHash: HashToken(secret),
		UserID:    userID,
		Purpose:   purpose,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}, nil
}
