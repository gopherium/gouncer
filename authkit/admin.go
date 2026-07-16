// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
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

type userSummary struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

// newUserSummary builds a userSummary from a user, normalizing the timestamp to UTC.
func newUserSummary(u gouncer.User) userSummary {
	return userSummary{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Disabled:  u.Disabled,
		CreatedAt: u.CreatedAt.UTC(),
	}
}

// List responds with every user account.
func (a *AdminHandlers) List(w http.ResponseWriter, r *http.Request) {
	users, err := a.store.ListUsers(r.Context())
	if err != nil {
		respondAuthError(w, err)
		return
	}
	summaries := make([]userSummary, len(users))
	for i, u := range users {
		summaries[i] = newUserSummary(u)
	}
	Respond(w, http.StatusOK, summaries)
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
	u, err := gouncer.NewUser(req.Email, req.Name, req.Password)
	if err != nil {
		respondAuthError(w, err)
		return
	}
	if err := a.store.CreateUser(r.Context(), u); err != nil {
		respondAuthError(w, err)
		return
	}
	Respond(w, http.StatusCreated, newUserSummary(u))
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
	if *req.Disabled && IdentityFromContext(r.Context()).ID == id {
		RespondError(w, http.StatusUnprocessableEntity, "cannot disable your own account")
		return
	}
	if err := a.store.SetUserDisabled(r.Context(), id, *req.Disabled); err != nil {
		respondAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
