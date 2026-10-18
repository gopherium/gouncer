// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// ErrUnknownRole reports a role the role source does not declare.
var ErrUnknownRole = errors.New("authkit: unknown role")

// ErrBeyondReach reports a role or an account that lies beyond the actor's reach.
var ErrBeyondReach = errors.New("authkit: role beyond reach")

// ErrRoleChanged reports an account whose role is no longer the one the write was checked against.
var ErrRoleChanged = errors.New("authkit: role changed")

// RoleSource answers which roles exist and which roles a caller may hand out.
type RoleSource interface {
	// Known reports whether the role exists.
	Known(role string) bool

	// Outranks reports whether the caller carries every capability the target carries.
	Outranks(caller, target string) bool
}

// declared reports whether the source declares the role, every role passing without a source.
func declared(source RoleSource, role string) bool {
	return source == nil || (role != "" && source.Known(role))
}

// refuseHandout reports whether it refused handing out an undeclared role or one the actor does not outrank.
func (a *AdminHandlers) refuseHandout(w http.ResponseWriter, r *http.Request, role string) bool {
	if a.roles == nil {
		return false
	}
	if !declared(a.roles, role) {
		respondAuthError(w, ErrUnknownRole)
		return true
	}
	return a.refuseOutranking(w, r, role)
}

// heldWithinReach returns the role the account holds and whether the actor may change the account.
func (a *AdminHandlers) heldWithinReach(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	reenable bool,
) (string, bool) {
	if a.roles == nil {
		return "", true
	}
	held, err := a.heldRole(r.Context(), id)
	if err != nil {
		respondAuthError(w, err)
		return "", false
	}
	if declared(a.roles, held) {
		return held, !a.refuseOutranking(w, r, held)
	}
	if reenable {
		respondBeyondReach(w)
	}
	return held, !reenable
}

// roleChangeHeld returns the role the account holds and whether the actor may move the account to role.
func (a *AdminHandlers) roleChangeHeld(
	w http.ResponseWriter,
	r *http.Request,
	id uuid.UUID,
	role string,
) (string, bool) {
	held, ok := a.heldWithinReach(w, r, id, false)
	return held, ok && !a.refuseHandout(w, r, role)
}

// refuseOutranking reports whether it refused an actor that does not outrank the role.
func (a *AdminHandlers) refuseOutranking(w http.ResponseWriter, r *http.Request, role string) bool {
	if a.roles.Outranks(IdentityFromContext(r.Context()).Role, role) {
		return false
	}
	respondBeyondReach(w)
	return true
}

// respondBeyondReach answers that the role lies beyond the actor's reach.
func respondBeyondReach(w http.ResponseWriter) {
	respondAuthError(w, ErrBeyondReach)
}

// accountReader is a store that reads one account by id.
type accountReader interface {
	UserByID(ctx context.Context, id uuid.UUID) (gouncer.User, error)
}

// heldRole returns the role the account holds, or gouncer.ErrUserNotFound when no account has the id.
func (a *AdminHandlers) heldRole(ctx context.Context, id uuid.UUID) (string, error) {
	if reader, ok := a.store.(accountReader); ok {
		u, err := reader.UserByID(ctx, id)
		return u.Role, err
	}
	users, err := a.store.ListUsers(ctx)
	if err != nil {
		return "", err
	}
	for _, u := range users {
		if u.ID == id {
			return u.Role, nil
		}
	}
	return "", gouncer.ErrUserNotFound
}
