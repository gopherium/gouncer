// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"net/http"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

// uuidZero is an account identifier no store holds.
const uuidZero = "00000000-0000-0000-0000-000000000000"

// privilegedServer mounts the admin routes gated on the given roles, signed in as the given role.
func privilegedServer(
	t *testing.T,
	store *testkit.Store,
	privileged gouncer.Roles,
	actorRole string,
) http.Handler {
	t.Helper()
	actor := addAda(t, store)
	actor.Role = actorRole
	store.Users[actor.ID] = actor
	cfg := authkit.Config{Store: store, Privileged: privileged}
	h := authkit.New(cfg)
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: store, Privileged: privileged})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.Handle("GET /api/users", h.RequireSession(http.HandlerFunc(admin.List)))
	mux.Handle("POST /api/users", h.RequireSession(http.HandlerFunc(admin.Create)))
	mux.Handle("PATCH /api/users/{id}", h.RequireSession(http.HandlerFunc(admin.SetDisabled)))
	mux.Handle("PUT /api/users/{id}/role", h.RequireSession(http.HandlerFunc(admin.SetRole)))
	return cookiedServer(mux, loginCookie(t, mux))
}

func TestAdminRoutesRefuseAnActorHoldingNoPrivilegedRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "editor")

	for _, held := range []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, "/api/users", ""},
		{http.MethodPost, "/api/users",
			`{"email":"maria@example.com","name":"Maria Perez","password":"correct horse battery"}`},
		{http.MethodPatch, "/api/users/" + uuidZero, `{"disabled":true}`},
		{http.MethodPut, "/api/users/" + uuidZero + "/role", `{"role":"admin"}`},
	} {
		recorder := doRequest(t, srv, held.method, held.target, held.body)

		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want %d", held.method, held.target, recorder.Code, http.StatusForbidden)
		}
		if code := decodeBody[authkit.ErrorResponse](t, recorder).Code; code != "role_insufficient" {
			t.Errorf("%s %s code = %q, want %q", held.method, held.target, code, "role_insufficient")
		}
	}
}

func TestAdminRoutesAdmitAnActorHoldingAPrivilegedRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "admin")

	recorder := doRequest(t, srv, http.MethodGet, "/api/users", "")

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAdminRoutesAdmitEveryoneWhenNoRolesAreConfigured(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, nil, "")

	recorder := doRequest(t, srv, http.MethodGet, "/api/users", "")

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d when the consumer configures no roles", recorder.Code, http.StatusOK)
	}
}

func TestListedAccountsCarryTheRoleTheyHold(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "admin")

	recorder := doRequest(t, srv, http.MethodGet, "/api/users", "")

	listed := decodeBody[[]map[string]any](t, recorder)
	if len(listed) != 1 {
		t.Fatalf("accounts = %d, want 1", len(listed))
	}
	if listed[0]["role"] != "admin" {
		t.Errorf("role = %v, want %q under the %q key", listed[0]["role"], "admin", "role")
	}
}

func TestCreatingAnAccountTakesTheRoleItIsGiven(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "admin")

	recorder := doRequest(t, srv, http.MethodPost, "/api/users",
		`{"email":"maria@example.com","name":"Maria Perez","password":"correct horse battery","role":"editor"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if role := decodeBody[authkit.Account](t, recorder).Role; role != "editor" {
		t.Errorf("role = %q, want %q", role, "editor")
	}
}

func TestSetRoleWritesTheRoleAnAccountHolds(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "admin")
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

	recorder := doRequest(t, srv, http.MethodPut, "/api/users/"+maria.ID.String()+"/role", `{"role":"editor"}`)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if held := store.Users[maria.ID].Role; held != "editor" {
		t.Errorf("role = %q, want %q", held, "editor")
	}
}

func TestSetRoleRefusesAnActorChangingItsOwnRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "admin")
	var actor gouncer.User
	for _, held := range store.Users {
		actor = held
	}

	recorder := doRequest(t, srv, http.MethodPut, "/api/users/"+actor.ID.String()+"/role", `{"role":"editor"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	if code := decodeBody[authkit.ErrorResponse](t, recorder).Code; code != "self_role_refused" {
		t.Errorf("code = %q, want %q", code, "self_role_refused")
	}
	if held := store.Users[actor.ID].Role; held != "admin" {
		t.Errorf("role = %q, want the refused write to leave %q", held, "admin")
	}
}

func TestSetRoleRefusesRemovingTheLastPrivilegedAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.SetRoleErr = gouncer.ErrLastPrivileged
	srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "admin")
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

	recorder := doRequest(t, srv, http.MethodPut, "/api/users/"+maria.ID.String()+"/role", `{"role":"editor"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	if code := decodeBody[authkit.ErrorResponse](t, recorder).Code; code != "last_privileged_refused" {
		t.Errorf("code = %q, want %q", code, "last_privileged_refused")
	}
}

func TestDisablingAnAccountGoesThroughTheGuardedWrite(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "admin")
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/users/"+maria.ID.String(), `{"disabled":true}`)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if store.DisabledUnderCover != 1 {
		t.Errorf("guarded disables = %d, want 1, the rail must be the path the handler takes", store.DisabledUnderCover)
	}
}

func TestDisablingRefusesRemovingTheLastPrivilegedAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.SetDisabledErr = gouncer.ErrLastPrivileged
	srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "admin")
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/users/"+maria.ID.String(), `{"disabled":true}`)

	if code := decodeBody[authkit.ErrorResponse](t, recorder).Code; code != "last_privileged_refused" {
		t.Errorf("code = %q, want %q", code, "last_privileged_refused")
	}
}

func TestGuardedWritesCarryTheConfiguredPrivilegedRoles(t *testing.T) {
	t.Parallel()

	for _, held := range []struct {
		name   string
		method string
		target func(id string) string
		body   string
	}{
		{"role", http.MethodPut, func(id string) string { return "/api/users/" + id + "/role" }, `{"role":"editor"}`},
		{"disable", http.MethodPatch, func(id string) string { return "/api/users/" + id }, `{"disabled":true}`},
	} {
		t.Run(held.name, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			srv := privilegedServer(t, store, gouncer.Roles{"admin"}, "admin")
			maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

			doRequest(t, srv, held.method, held.target(maria.ID.String()), held.body)

			if len(store.CoverGiven) != 1 || store.CoverGiven[0] != "admin" {
				t.Errorf("cover handed to the guard = %v, want the configured roles, or the rail runs uncovered",
					store.CoverGiven)
			}
		})
	}
}
