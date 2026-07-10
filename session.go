// SPDX-License-Identifier: Apache-2.0

package gouncer

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DefaultSessionDuration is the lifetime [NewSession] applies.
const DefaultSessionDuration = 30 * 24 * time.Hour

const sessionTokenBytes = 32

// Session is a login session. Build one with [NewSession]. Token is
// handed to the client once, only TokenHash is persisted.
type Session struct {
	Token     string
	TokenHash []byte
	UserID    uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewSession issues a session for the user with a fresh random token.
func NewSession(userID uuid.UUID) (Session, error) {
	raw := make([]byte, sessionTokenBytes)
	if _, err := randRead(raw); err != nil {
		return Session{}, fmt.Errorf("gouncer: generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()
	return Session{
		Token:     token,
		TokenHash: HashToken(token),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(DefaultSessionDuration),
	}, nil
}

// HashToken returns the digest under which a session token is persisted
// and looked up.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
