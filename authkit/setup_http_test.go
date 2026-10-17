// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

// setupRoute is the path the setup handlers are reached at in these tests.
const setupRoute = "/api/auth/setup"

// codeRoute is the path the code check is reached at in these tests.
const codeRoute = "/api/auth/setup/code"

// countingStore is a testkit store that counts the times it is asked whether it holds an account.
type countingStore struct {
	*testkit.Store
	// asked counts the HasUsers calls.
	asked atomic.Int32
}

// HasUsers counts the call and answers as the testkit store does.
func (c *countingStore) HasUsers(ctx context.Context) (bool, error) {
	c.asked.Add(1)
	return c.Store.HasUsers(ctx)
}

// firstAccountBody returns the JSON body that creates Maria Perez with code and password.
func firstAccountBody(code, password string) string {
	return fmt.Sprintf(`{"code":%q,"email":"maria@example.com","name":"Maria Perez","password":%q}`, code, password)
}

// codeBody returns the JSON body that carries code to the code check.
func codeBody(code string) string {
	return fmt.Sprintf(`{"code":%q}`, code)
}

// unstored fails t unless the response tells caches to keep nothing.
func unstored(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

// refusedWith fails t unless the response carries status and the error code.
func refusedWith(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) errorBody {
	t.Helper()
	if recorder.Code != status {
		t.Errorf("status = %d, want %d, body %s", recorder.Code, status, recorder.Body.String())
	}
	body := decodeError(t, recorder)
	if body.Code != code {
		t.Errorf("code = %q, want %q", body.Code, code)
	}
	return body
}

// TestStatusReportsWhetherTheSiteWaitsForItsFirstAccount checks the public probe for each kind of site.
func TestStatusReportsWhetherTheSiteWaitsForItsFirstAccount(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		store func(t *testing.T) authkit.SetupStore
		want  string
	}{
		{"an empty store", func(*testing.T) authkit.SetupStore { return testkit.NewStore() }, `{"open":true}`},
		{"a store with an account", func(t *testing.T) authkit.SetupStore {
			store := testkit.NewStore()
			addMaria(t, store)
			return store
		}, `{"open":false}`},
		{"no store", func(*testing.T) authkit.SetupStore { return nil }, `{"open":true}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := newSetup(codePath(t)).Status(tt.store(t))

			recorder := doRequest(t, handler, http.MethodGet, setupRoute, "")

			if recorder.Code != http.StatusOK || recorder.Body.String() != tt.want {
				t.Errorf("Status answered %d %s, want 200 %s", recorder.Code, recorder.Body.String(), tt.want)
			}
			unstored(t, recorder)
		})
	}
}

// TestStatusStopsReadingTheStoreOnceTheSiteIsSetUp checks that a set-up site's probe leaves the store alone.
func TestStatusStopsReadingTheStoreOnceTheSiteIsSetUp(t *testing.T) {
	t.Parallel()

	store := &countingStore{Store: testkit.NewStore()}
	addMaria(t, store.Store)
	handler := newSetup(codePath(t)).Status(store)
	if first := doRequest(t, handler, http.MethodGet, setupRoute, ""); first.Body.String() != `{"open":false}` {
		t.Fatalf("the first Status answered %s, want closed", first.Body.String())
	}
	asked := store.asked.Load()

	for range 3 {
		if again := doRequest(t, handler, http.MethodGet, setupRoute, ""); again.Body.String() != `{"open":false}` {
			t.Errorf("a later Status answered %s, want closed", again.Body.String())
		}
	}

	if more := store.asked.Load() - asked; more != 0 {
		t.Errorf("the later probes asked the store %d times, want none", more)
	}
}

// TestStatusAnswersInternalWhenTheStoreFails checks the probe over a store that cannot be read.
func TestStatusAnswersInternalWhenTheStoreFails(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.LookupErr = errLookupFailed

	recorder := doRequest(t, newSetup(codePath(t)).Status(store), http.MethodGet, setupRoute, "")

	refusedWith(t, recorder, http.StatusInternalServerError, "internal")
	unstored(t, recorder)
}

// TestStatusAnswersClosedWithoutAStoreOnceTheFirstAccountExists checks the probe of a site this process set up.
func TestStatusAnswersClosedWithoutAStoreOnceTheFirstAccountExists(t *testing.T) {
	t.Parallel()

	setup, _, code := prepared(t)
	if _, err := firstMaria(t, setup, testkit.NewStore(), code); err != nil {
		t.Fatalf("CreateFirstAccount() error = %v, want nil", err)
	}

	recorder := doRequest(t, setup.Status(nil), http.MethodGet, setupRoute, "")

	if recorder.Code != http.StatusOK || recorder.Body.String() != `{"open":false}` {
		t.Errorf("Status answered %d %s, want 200 and closed", recorder.Code, recorder.Body.String())
	}
	unstored(t, recorder)
}

// TestCreateAnswersTheAccountWithoutASession checks the created first account and the missing cookie.
func TestCreateAnswersTheAccountWithoutASession(t *testing.T) {
	t.Parallel()

	setup, _, code := prepared(t)

	recorder := doRequest(t, setup.Create(testkit.NewStore()), http.MethodPost, setupRoute,
		firstAccountBody(code, testPassword))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body %s", recorder.Code, recorder.Body.String())
	}
	account := decodeBody[authkit.Account](t, recorder)
	want := authkit.Account{
		ID: account.ID, Email: "maria@example.com", Name: "Maria Perez", Confirmed: true, Role: "owner",
		CreatedAt: account.CreatedAt,
	}
	if account != want {
		t.Errorf("Create answered %+v, want %+v", account, want)
	}
	if cookies := recorder.Header().Values("Set-Cookie"); len(cookies) != 0 {
		t.Errorf("Set-Cookie = %q, want no session", cookies)
	}
	unstored(t, recorder)
}

// TestCreateRefusesWhatItCannotTake checks each refusal of the first account route.
func TestCreateRefusesWhatItCannotTake(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		shape  func(t *testing.T, path string, store *testkit.Store)
		body   func(code string) string
		status int
		code   string
		meta   map[string]any
	}{
		{"a wrong code", nil, func(string) string { return firstAccountBody(rand.Text(), testPassword) },
			http.StatusUnauthorized, "setup_code_invalid", nil},
		{"an expired code", func(t *testing.T, path string, _ *testkit.Store) { expire(t, path) },
			func(code string) string { return firstAccountBody(code, testPassword) },
			http.StatusUnauthorized, "setup_code_expired", nil},
		{"a short password", nil, func(code string) string { return firstAccountBody(code, "elevenchars") },
			http.StatusUnprocessableEntity, "password_too_short", map[string]any{"min": float64(12)}},
		{"a bad email", nil, func(code string) string {
			return fmt.Sprintf(`{"code":%q,"email":"maria at example.com","name":"Maria Perez","password":%q}`,
				code, testPassword)
		}, http.StatusUnprocessableEntity, "email_invalid", nil},
		{"a body that is not JSON", nil, func(string) string { return `{"code":` },
			http.StatusBadRequest, "body_malformed", nil},
		{"a body over one mebibyte", nil, func(code string) string {
			return `{"code":"` + code + strings.Repeat(" ", authkit.MaxRequestBodyBytes) + `"}`
		}, http.StatusRequestEntityTooLarge, "body_too_large", map[string]any{"max": float64(authkit.MaxRequestBodyBytes)}},
		{"a set-up site", func(t *testing.T, _ string, store *testkit.Store) { addMaria(t, store) },
			func(code string) string { return firstAccountBody(code, testPassword) },
			http.StatusConflict, "setup_done", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			setup, path, code := prepared(t)
			store := testkit.NewStore()
			if tt.shape != nil {
				tt.shape(t, path, store)
			}

			recorder := doRequest(t, setup.Create(store), http.MethodPost, setupRoute, tt.body(code))

			body := refusedWith(t, recorder, tt.status, tt.code)
			for key, want := range tt.meta {
				if body.Meta[key] != want {
					t.Errorf("meta = %v, want %s = %v", body.Meta, key, want)
				}
			}
			unstored(t, recorder)
		})
	}
}

// TestCreateAnswersSetupDoneBeforeItReadsTheBody checks that a set-up site refuses before it decodes.
func TestCreateAnswersSetupDoneBeforeItReadsTheBody(t *testing.T) {
	t.Parallel()

	setup, _, _ := prepared(t)
	store := testkit.NewStore()
	addMaria(t, store)

	recorder := doRequest(t, setup.Create(store), http.MethodPost, setupRoute, "not json")

	refusedWith(t, recorder, http.StatusConflict, "setup_done")
}

// TestCreateIgnoresARoleInTheBody checks that the first role comes from the config alone.
func TestCreateIgnoresARoleInTheBody(t *testing.T) {
	t.Parallel()

	setup, _, code := prepared(t)
	body := fmt.Sprintf(`{"code":%q,"email":"maria@example.com","name":"Maria Perez","password":%q,"role":"author"}`,
		code, testPassword)

	recorder := doRequest(t, setup.Create(testkit.NewStore()), http.MethodPost, setupRoute, body)

	if account := decodeBody[authkit.Account](t, recorder); recorder.Code != http.StatusCreated ||
		account.Role != "owner" {
		t.Errorf("Create answered %d with the role %q, want 201 and owner", recorder.Code, account.Role)
	}
}

// TestCreateAnswersInternalForARoleItCannotGive checks that a misconfigured role never answers role_unknown.
func TestCreateAnswersInternalForARoleItCannotGive(t *testing.T) {
	t.Parallel()

	_, path, code := prepared(t)
	cfg := setupConfig(path)
	cfg.Role = "editor"

	recorder := doRequest(t, authkit.NewSetup(cfg).Create(testkit.NewStore()), http.MethodPost, setupRoute,
		firstAccountBody(code, testPassword))

	refusedWith(t, recorder, http.StatusInternalServerError, "internal")
}

// TestCheckCodeAnswersEachState checks each answer of the code check route.
func TestCheckCodeAnswersEachState(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		shape  func(t *testing.T, path string, store *testkit.Store)
		body   func(code string) string
		status int
		code   string
	}{
		{"a live code", nil, codeBody, http.StatusNoContent, ""},
		{"a wrong code", nil, func(string) string { return codeBody(rand.Text()) },
			http.StatusUnauthorized, "setup_code_invalid"},
		{"an expired code", func(t *testing.T, path string, _ *testkit.Store) { expire(t, path) }, codeBody,
			http.StatusUnauthorized, "setup_code_expired"},
		{"a file that holds no code", func(t *testing.T, path string, _ *testkit.Store) {
			plantFile(t, path, "hello\n", 0o600)
		}, codeBody, http.StatusInternalServerError, "setup_code_file"},
		{"a body that is not JSON", nil, func(string) string { return `{"code":` },
			http.StatusBadRequest, "body_malformed"},
		{"a set-up site, before the body is read", func(t *testing.T, _ string, store *testkit.Store) {
			addMaria(t, store)
		}, func(string) string { return "not json" }, http.StatusConflict, "setup_done"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			setup, path, code := prepared(t)
			store := testkit.NewStore()
			if tt.shape != nil {
				tt.shape(t, path, store)
			}

			recorder := doRequest(t, setup.CheckCode(store), http.MethodPost, codeRoute, tt.body(code))

			if tt.status == http.StatusNoContent {
				if recorder.Code != http.StatusNoContent || recorder.Body.Len() != 0 {
					t.Errorf("CheckCode answered %d %q, want 204 with no body", recorder.Code, recorder.Body.String())
				}
			} else {
				refusedWith(t, recorder, tt.status, tt.code)
			}
			unstored(t, recorder)
		})
	}
}

// TestCheckCodeSpendsNothing checks that a code the check accepted still creates the first account.
func TestCheckCodeSpendsNothing(t *testing.T) {
	t.Parallel()

	setup, _, code := prepared(t)
	store := testkit.NewStore()
	if checked := doRequest(t, setup.CheckCode(store), http.MethodPost, codeRoute, codeBody(code)); checked.Code !=
		http.StatusNoContent {
		t.Fatalf("CheckCode answered %d, want 204", checked.Code)
	}

	recorder := doRequest(t, setup.Create(store), http.MethodPost, setupRoute, firstAccountBody(code, testPassword))

	if recorder.Code != http.StatusCreated {
		t.Errorf("Create after the check answered %d %s, want 201", recorder.Code, recorder.Body.String())
	}
}
