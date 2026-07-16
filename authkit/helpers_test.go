// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

var (
	_ gouncer.Store      = (*testkit.Store)(nil)
	_ authkit.AdminStore = (*testkit.Store)(nil)
)

const testPassword = "correct horse battery"

const testCookieName = "__Host-session"

type errorBody struct {
	Error string `json:"error"`
}

// addAda stores the default test user.
func addAda(t *testing.T, store *testkit.Store) gouncer.User {
	t.Helper()
	return store.AddUser(t, "ada@example.com", "Ada Lovelace", testPassword)
}

// newAuthServer mounts the session and admin handlers on their canonical routes.
func newAuthServer(store *testkit.Store) http.Handler {
	return newConfiguredServer(authkit.Config{Store: store}, store)
}

// newConfiguredServer mounts the handlers built from cfg, administering store.
func newConfiguredServer(cfg authkit.Config, store *testkit.Store) http.Handler {
	h := authkit.New(cfg)
	admin := authkit.NewAdmin(store)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.HandleFunc("POST /api/auth/logout", h.Logout)
	mux.Handle("GET /api/auth/session", h.RequireSession(http.HandlerFunc(h.Session)))
	mux.Handle("GET /api/users", h.RequireSession(http.HandlerFunc(admin.List)))
	mux.Handle("POST /api/users", h.RequireSession(http.HandlerFunc(admin.Create)))
	mux.Handle("PATCH /api/users/{id}", h.RequireSession(http.HandlerFunc(admin.SetDisabled)))
	return mux
}

// doRequest performs a JSON request against handler, returning the recorded response.
func doRequest(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, target, reader)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// doLogin posts credentials to the login route.
func doLogin(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doRequest(t, handler, http.MethodPost, "/api/auth/login", body)
}

// decodeBody unmarshals the recorded response body into a value of type T.
func decodeBody[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(recorder.Body.Bytes(), &v); err != nil {
		t.Fatalf("decoding %q: %v", recorder.Body.String(), err)
	}
	return v
}

// sessionCookie returns the response's session cookie under name, failing without one.
func sessionCookie(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("no %s cookie in the response", name)
	return nil
}

// loginCookie logs the default test user in and returns the issued cookie.
func loginCookie(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	recorder := doLogin(t, handler, `{"email":"ada@example.com","password":"correct horse battery"}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", recorder.Code, http.StatusOK)
	}
	return sessionCookie(t, recorder, testCookieName)
}
