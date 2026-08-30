// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// AdminStore persists users for both login and administration.
type AdminStore interface {
	gouncer.Store

	// ListUsers returns every user account ordered for display.
	ListUsers(ctx context.Context) ([]gouncer.User, error)

	// SetUserDisabledUnderCover updates whether the account may log in, under the privileged guard.
	SetUserDisabledUnderCover(ctx context.Context, id uuid.UUID, disabled bool, privileged gouncer.Roles) error

	// SetUserRole writes the role an account holds, under the privileged guard.
	SetUserRole(ctx context.Context, id uuid.UUID, role string, privileged gouncer.Roles) error
}

// AdminConfig parameterizes account administration.
type AdminConfig struct {
	// Store persists the accounts administered.
	Store AdminStore
	// Privileged names the roles an administrator holds. Empty admits every role.
	Privileged gouncer.Roles
}

// AdminHandlers serves user administration over HTTP. Mount its handlers
// behind RequireSession.
type AdminHandlers struct {
	store      AdminStore
	privileged gouncer.Roles
}

// NewAdmin returns AdminHandlers administering the accounts cfg names.
func NewAdmin(cfg AdminConfig) *AdminHandlers {
	return &AdminHandlers{store: cfg.Store, privileged: slices.Clone(cfg.Privileged)}
}

// refuseUnprivileged reports whether it refused an actor holding no privileged role.
func (a *AdminHandlers) refuseUnprivileged(w http.ResponseWriter, r *http.Request) bool {
	if len(a.privileged) == 0 || a.privileged.Holds(IdentityFromContext(r.Context()).Role) {
		return false
	}
	RespondError(w, http.StatusForbidden, ErrorResponse{Message: "role insufficient", Code: "role_insufficient"})
	return true
}

// Account is one user account as administration reports it.
type Account struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Disabled  bool      `json:"disabled"`
	Confirmed bool      `json:"confirmed"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// newAccount builds an Account from a user, normalizing the timestamp to UTC.
func newAccount(u gouncer.User) Account {
	return Account{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Disabled:  u.Disabled,
		Confirmed: u.Confirmed,
		Role:      u.Role,
		CreatedAt: u.CreatedAt.UTC(),
	}
}

// ErrSelfDisable reports an account disabling itself.
var ErrSelfDisable = errors.New("authkit: cannot disable your own account")

// ListAccounts returns every user account ordered for display.
func (a *AdminHandlers) ListAccounts(ctx context.Context) ([]Account, error) {
	users, err := a.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	accounts := make([]Account, len(users))
	for i, u := range users {
		accounts[i] = newAccount(u)
	}
	return accounts, nil
}

// CreateAccount validates and persists a new user account.
func (a *AdminHandlers) CreateAccount(ctx context.Context, email, name, password, role string) (Account, error) {
	u, err := gouncer.NewUser(email, name, password)
	if err != nil {
		return Account{}, err
	}
	u.Role = role
	if err := a.store.CreateUser(ctx, u); err != nil {
		return Account{}, err
	}
	return newAccount(u), nil
}

// SetAccountDisabled updates whether the account may log in, refusing an actor disabling itself.
func (a *AdminHandlers) SetAccountDisabled(ctx context.Context, actorID, id uuid.UUID, disabled bool) error {
	if disabled && actorID == id {
		return ErrSelfDisable
	}
	return a.store.SetUserDisabledUnderCover(ctx, id, disabled, a.privileged)
}

// ErrSelfRole reports an account changing its own role.
var ErrSelfRole = errors.New("authkit: cannot change your own role")

// SetAccountRole writes the role an account holds, refusing an actor changing its own.
func (a *AdminHandlers) SetAccountRole(ctx context.Context, actorID, id uuid.UUID, role string) error {
	if actorID == id {
		return ErrSelfRole
	}
	return a.store.SetUserRole(ctx, id, role, a.privileged)
}

// List responds with every user account.
func (a *AdminHandlers) List(w http.ResponseWriter, r *http.Request) {
	if a.refuseUnprivileged(w, r) {
		return
	}
	accounts, err := a.ListAccounts(r.Context())
	if err != nil {
		respondAuthError(w, err)
		return
	}
	Respond(w, http.StatusOK, accounts)
}

// Create decodes credentials, creates a user account, persists it, and
// responds with the created account.
func (a *AdminHandlers) Create(w http.ResponseWriter, r *http.Request) {
	if a.refuseUnprivileged(w, r) {
		return
	}
	type request struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	req, err := Decode[request](w, r)
	if err != nil {
		respondDecodeError(w, err, "malformed json")
		return
	}
	account, err := a.CreateAccount(r.Context(), req.Email, req.Name, req.Password, req.Role)
	if err != nil {
		respondAuthError(w, err)
		return
	}
	Respond(w, http.StatusCreated, account)
}

// SetDisabled parses the user id from the request's "id" path value and
// updates whether that account may log in, refusing to disable the requester.
func (a *AdminHandlers) SetDisabled(w http.ResponseWriter, r *http.Request) {
	if a.refuseUnprivileged(w, r) {
		return
	}
	type request struct {
		Disabled *bool `json:"disabled"`
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, ErrorResponse{
			Message: "malformed user id", Code: "user_id_malformed",
		})
		return
	}
	req, err := Decode[request](w, r)
	if err != nil {
		respondDecodeError(w, err, "malformed json")
		return
	}
	if req.Disabled == nil {
		RespondError(w, http.StatusUnprocessableEntity, ErrorResponse{
			Message: "disabled is required", Code: "body_field_required", Meta: map[string]any{"field": "disabled"},
		})
		return
	}
	actor := IdentityFromContext(r.Context())
	err = a.SetAccountDisabled(r.Context(), actor.ID, id, *req.Disabled)
	if errors.Is(err, ErrSelfDisable) {
		RespondError(w, http.StatusUnprocessableEntity, ErrorResponse{
			Message: "cannot disable your own account", Code: "self_disable_refused",
		})
		return
	}
	if a.refuseLastPrivileged(w, err) {
		return
	}
	if err != nil {
		respondAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// refuseLastPrivileged reports whether it refused a write leaving no enabled account privileged.
func (a *AdminHandlers) refuseLastPrivileged(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, gouncer.ErrLastPrivileged) {
		return false
	}
	RespondError(w, http.StatusUnprocessableEntity, ErrorResponse{
		Message: "the last privileged account must remain", Code: "last_privileged_refused",
	})
	return true
}

// SetRole parses the account id from the request "id" path value and writes the role it holds.
func (a *AdminHandlers) SetRole(w http.ResponseWriter, r *http.Request) {
	if a.refuseUnprivileged(w, r) {
		return
	}
	type request struct {
		Role *string `json:"role"`
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, ErrorResponse{
			Message: "malformed user id", Code: "user_id_malformed",
		})
		return
	}
	req, err := Decode[request](w, r)
	if err != nil {
		respondDecodeError(w, err, "malformed json")
		return
	}
	if req.Role == nil {
		RespondError(w, http.StatusUnprocessableEntity, ErrorResponse{
			Message: "role is required", Code: "body_field_required", Meta: map[string]any{"field": "role"},
		})
		return
	}
	actor := IdentityFromContext(r.Context())
	err = a.SetAccountRole(r.Context(), actor.ID, id, *req.Role)
	if errors.Is(err, ErrSelfRole) {
		RespondError(w, http.StatusUnprocessableEntity, ErrorResponse{
			Message: "cannot change your own role", Code: "self_role_refused",
		})
		return
	}
	if a.refuseLastPrivileged(w, err) {
		return
	}
	if err != nil {
		respondAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
