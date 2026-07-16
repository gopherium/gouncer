// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"

	"github.com/google/uuid"
)

// Identity is the authenticated user exposed to handlers, deliberately
// excluding credentials such as the password hash.
type Identity struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

type contextKey int

const identityContextKey contextKey = 0

// WithIdentity returns a context carrying the authenticated user's identity.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityContextKey, id)
}

// IdentityFromContext returns the identity stored by the RequireSession
// middleware, or the zero identity outside of it.
func IdentityFromContext(ctx context.Context) Identity {
	id, _ := ctx.Value(identityContextKey).(Identity)
	return id
}
