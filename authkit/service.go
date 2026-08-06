// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// ErrInvalidCredentials reports a login that names no enabled account.
var ErrInvalidCredentials = errors.New("authkit: invalid credentials")

// Authenticate verifies credentials, answering the account identity or [ErrInvalidCredentials].
func (h *Handlers) Authenticate(ctx context.Context, email, password string) (Identity, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	u, err := h.store.UserByEmail(ctx, normalized)
	if errors.Is(err, gouncer.ErrUserNotFound) {
		gouncer.VerifyPassword(dummyPasswordHash, password)
		return Identity{}, ErrInvalidCredentials
	}
	if err != nil {
		return Identity{}, err
	}
	if !gouncer.VerifyPassword(u.PasswordHash, password) || u.Disabled {
		return Identity{}, ErrInvalidCredentials
	}
	return Identity{ID: u.ID, Email: u.Email, Name: u.Name}, nil
}

// StartSession issues and persists a session for userID, returning its cookie.
func (h *Handlers) StartSession(ctx context.Context, userID uuid.UUID) (*http.Cookie, error) {
	session, err := h.newSession(userID)
	if err != nil {
		return nil, err
	}
	session.ExpiresAt = session.CreatedAt.Add(h.ttl)
	if err := h.store.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	return sessionCookie(h.cookieName, session.Token, int(h.ttl.Seconds())), nil
}

// EndSession deletes the session behind token, returning the clearing cookie.
func (h *Handlers) EndSession(ctx context.Context, token string) (*http.Cookie, error) {
	if token != "" {
		if err := h.store.DeleteSession(ctx, gouncer.HashToken(token)); err != nil {
			return nil, err
		}
	}
	return sessionCookie(h.cookieName, "", -1), nil
}

// SessionIdentity resolves the identity behind a session token.
func (h *Handlers) SessionIdentity(ctx context.Context, token string) (Identity, error) {
	u, err := h.store.UserBySession(ctx, gouncer.HashToken(token), time.Now().UTC())
	if err != nil {
		return Identity{}, err
	}
	return Identity{ID: u.ID, Email: u.Email, Name: u.Name}, nil
}

// CookieName reports the configured session cookie name.
func (h *Handlers) CookieName() string {
	return h.cookieName
}
