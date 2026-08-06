// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// AdminStore persists users for both login and administration.
type AdminStore interface {
	gouncer.Store

	// ListUsers returns every user account ordered for display.
	ListUsers(ctx context.Context) ([]gouncer.User, error)

	// SetUserDisabled updates whether the account may log in.
	SetUserDisabled(ctx context.Context, id uuid.UUID, disabled bool) error
}

// AdminHandlers serves user administration over HTTP. Mount its handlers
// behind RequireSession.
type AdminHandlers struct {
	store AdminStore
}

// NewAdmin returns AdminHandlers administering the accounts in store.
func NewAdmin(store AdminStore) *AdminHandlers {
	return &AdminHandlers{store: store}
}

// Account is one user account as administration reports it.
type Account struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

// newAccount builds an Account from a user, normalizing the timestamp to UTC.
func newAccount(u gouncer.User) Account {
	return Account{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Disabled:  u.Disabled,
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
func (a *AdminHandlers) CreateAccount(ctx context.Context, email, name, password string) (Account, error) {
	u, err := gouncer.NewUser(email, name, password)
	if err != nil {
		return Account{}, err
	}
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
	return a.store.SetUserDisabled(ctx, id, disabled)
}

// List responds with every user account.
func (a *AdminHandlers) List(w http.ResponseWriter, r *http.Request) {
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
	type request struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	req, err := Decode[request](w, r)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "malformed json")
		return
	}
	account, err := a.CreateAccount(r.Context(), req.Email, req.Name, req.Password)
	if err != nil {
		respondAuthError(w, err)
		return
	}
	Respond(w, http.StatusCreated, account)
}

// SetDisabled parses the user id from the request's "id" path value and
// updates whether that account may log in, refusing to disable the requester.
func (a *AdminHandlers) SetDisabled(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Disabled *bool `json:"disabled"`
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "malformed user id")
		return
	}
	req, err := Decode[request](w, r)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "malformed json")
		return
	}
	if req.Disabled == nil {
		RespondError(w, http.StatusUnprocessableEntity, "disabled is required")
		return
	}
	actor := IdentityFromContext(r.Context())
	err = a.SetAccountDisabled(r.Context(), actor.ID, id, *req.Disabled)
	if errors.Is(err, ErrSelfDisable) {
		RespondError(w, http.StatusUnprocessableEntity, "cannot disable your own account")
		return
	}
	if err != nil {
		respondAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
