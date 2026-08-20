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

// privilegedServer mounts the admin routes gated on the given ranks, signed in as the given rank.
func privilegedServer(
	t *testing.T,
	store *testkit.Store,
	privileged gouncer.Ranks,
	actorRank string,
) http.Handler {
	t.Helper()
	actor := addAda(t, store)
	actor.Rank = actorRank
	store.Users[actor.ID] = actor
	cfg := authkit.Config{Store: store, Privileged: privileged}
	h := authkit.New(cfg)
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: store, Privileged: privileged})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.Handle("GET /api/users", h.RequireSession(http.HandlerFunc(admin.List)))
	mux.Handle("POST /api/users", h.RequireSession(http.HandlerFunc(admin.Create)))
	mux.Handle("PATCH /api/users/{id}", h.RequireSession(http.HandlerFunc(admin.SetDisabled)))
	mux.Handle("PUT /api/users/{id}/rank", h.RequireSession(http.HandlerFunc(admin.SetRank)))
	return cookiedServer(mux, loginCookie(t, mux))
}

func TestAdminRoutesRefuseAnActorHoldingNoPrivilegedRank(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Ranks{"admin"}, "editor")

	for _, held := range []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, "/api/users", ""},
		{http.MethodPost, "/api/users",
			`{"email":"maria@example.com","name":"Maria Perez","password":"correct horse battery"}`},
		{http.MethodPatch, "/api/users/" + uuidZero, `{"disabled":true}`},
		{http.MethodPut, "/api/users/" + uuidZero + "/rank", `{"rank":"admin"}`},
	} {
		recorder := doRequest(t, srv, held.method, held.target, held.body)

		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s %s status = %d, want %d", held.method, held.target, recorder.Code, http.StatusForbidden)
		}
		if code := decodeBody[authkit.Refusal](t, recorder).Code; code != "rank_insufficient" {
			t.Errorf("%s %s code = %q, want %q", held.method, held.target, code, "rank_insufficient")
		}
	}
}

func TestAdminRoutesAdmitAnActorHoldingAPrivilegedRank(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Ranks{"admin"}, "admin")

	recorder := doRequest(t, srv, http.MethodGet, "/api/users", "")

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAdminRoutesAdmitEveryoneWhenNoRanksAreConfigured(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, nil, "")

	recorder := doRequest(t, srv, http.MethodGet, "/api/users", "")

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d when the consumer configures no ranks", recorder.Code, http.StatusOK)
	}
}

func TestListedAccountsCarryTheRankTheyHold(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Ranks{"admin"}, "admin")

	recorder := doRequest(t, srv, http.MethodGet, "/api/users", "")

	listed := decodeBody[[]authkit.Account](t, recorder)
	if len(listed) != 1 {
		t.Fatalf("accounts = %d, want 1", len(listed))
	}
	if listed[0].Rank != "admin" {
		t.Errorf("rank = %q, want %q", listed[0].Rank, "admin")
	}
}

func TestCreatingAnAccountTakesTheRankItIsGiven(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Ranks{"admin"}, "admin")

	recorder := doRequest(t, srv, http.MethodPost, "/api/users",
		`{"email":"maria@example.com","name":"Maria Perez","password":"correct horse battery","rank":"editor"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if rank := decodeBody[authkit.Account](t, recorder).Rank; rank != "editor" {
		t.Errorf("rank = %q, want %q", rank, "editor")
	}
}

func TestSetRankWritesTheRankAnAccountHolds(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Ranks{"admin"}, "admin")
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

	recorder := doRequest(t, srv, http.MethodPut, "/api/users/"+maria.ID.String()+"/rank", `{"rank":"editor"}`)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if held := store.Users[maria.ID].Rank; held != "editor" {
		t.Errorf("rank = %q, want %q", held, "editor")
	}
}

func TestSetRankRefusesAnActorChangingItsOwnRank(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Ranks{"admin"}, "admin")
	var actor gouncer.User
	for _, held := range store.Users {
		actor = held
	}

	recorder := doRequest(t, srv, http.MethodPut, "/api/users/"+actor.ID.String()+"/rank", `{"rank":"editor"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	if code := decodeBody[authkit.Refusal](t, recorder).Code; code != "self_rank_refused" {
		t.Errorf("code = %q, want %q", code, "self_rank_refused")
	}
	if held := store.Users[actor.ID].Rank; held != "admin" {
		t.Errorf("rank = %q, want the refused write to leave %q", held, "admin")
	}
}

func TestSetRankRefusesRemovingTheLastPrivilegedAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.SetRankErr = gouncer.ErrLastPrivileged
	srv := privilegedServer(t, store, gouncer.Ranks{"admin"}, "admin")
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

	recorder := doRequest(t, srv, http.MethodPut, "/api/users/"+maria.ID.String()+"/rank", `{"rank":"editor"}`)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnprocessableEntity)
	}
	if code := decodeBody[authkit.Refusal](t, recorder).Code; code != "last_privileged_refused" {
		t.Errorf("code = %q, want %q", code, "last_privileged_refused")
	}
}

func TestDisablingAnAccountGoesThroughTheGuardedWrite(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	srv := privilegedServer(t, store, gouncer.Ranks{"admin"}, "admin")
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
	srv := privilegedServer(t, store, gouncer.Ranks{"admin"}, "admin")
	maria := store.AddUser(t, "maria@example.com", "Maria Perez", testPassword)

	recorder := doRequest(t, srv, http.MethodPatch, "/api/users/"+maria.ID.String(), `{"disabled":true}`)

	if code := decodeBody[authkit.Refusal](t, recorder).Code; code != "last_privileged_refused" {
		t.Errorf("code = %q, want %q", code, "last_privileged_refused")
	}
}
