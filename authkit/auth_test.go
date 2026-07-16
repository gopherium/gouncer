// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestLoginIssuesASessionCookie(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAda(t, store)
	handler := newAuthServer(store)

	recorder := doLogin(t, handler, `{"email":" ADA@Example.com ","password":"correct horse battery"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	cookie := sessionCookie(t, recorder, testCookieName)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Errorf("cookie = %+v, want HttpOnly, Secure, SameSite=Lax, Path=/", cookie)
	}
	session, ok := store.Sessions[string(gouncer.HashToken(cookie.Value))]
	if !ok {
		t.Fatal("no session persisted for the issued cookie token")
	}
	if got := session.ExpiresAt.Sub(session.CreatedAt); got != gouncer.DefaultSessionDuration {
		t.Errorf("session lifetime = %v, want %v", got, gouncer.DefaultSessionDuration)
	}
	body := decodeBody[map[string]any](t, recorder)
	if body["email"] != "ada@example.com" || body["name"] != "Ada Lovelace" {
		t.Errorf("body = %v, want the logged-in user's email and name", body)
	}
	if _, exposed := body["password_hash"]; exposed {
		t.Error("response exposes password_hash")
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		configure func(t *testing.T, store *testkit.Store)
		body      string
		want      int
	}{
		"wrong password": {
			configure: func(t *testing.T, store *testkit.Store) {
				addAda(t, store)
			},
			body: `{"email":"ada@example.com","password":"wrong password!"}`,
			want: http.StatusUnauthorized,
		},
		"unknown email": {
			configure: func(t *testing.T, store *testkit.Store) {},
			body:      `{"email":"nobody@example.com","password":"correct horse battery"}`,
			want:      http.StatusUnauthorized,
		},
		"disabled user": {
			configure: func(t *testing.T, store *testkit.Store) {
				u := addAda(t, store)
				u.Disabled = true
				store.Users[u.ID] = u
			},
			body: `{"email":"ada@example.com","password":"correct horse battery"}`,
			want: http.StatusUnauthorized,
		},
		"malformed body": {
			configure: func(t *testing.T, store *testkit.Store) {},
			body:      `{"email":`,
			want:      http.StatusBadRequest,
		},
		"trailing content": {
			configure: func(t *testing.T, store *testkit.Store) {},
			body:      `{"email":"ada@example.com","password":"correct horse battery"}{}`,
			want:      http.StatusBadRequest,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			tc.configure(t, store)
			handler := newAuthServer(store)

			recorder := doLogin(t, handler, tc.body)

			if recorder.Code != tc.want {
				t.Errorf("status = %d, want %d", recorder.Code, tc.want)
			}
			if recorder.Code != http.StatusOK {
				for _, cookie := range recorder.Result().Cookies() {
					if cookie.Name == testCookieName && cookie.MaxAge >= 0 {
						t.Error("failed login issued a session cookie")
					}
				}
			}
		})
	}
}

func TestLoginReportsStoreFailures(t *testing.T) {
	t.Parallel()

	t.Run("lookup failure", func(t *testing.T) {
		t.Parallel()

		store := testkit.NewStore()
		store.LookupErr = context.DeadlineExceeded
		handler := newAuthServer(store)

		recorder := doLogin(t, handler, `{"email":"ada@example.com","password":"correct horse battery"}`)

		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
	})

	t.Run("session creation failure", func(t *testing.T) {
		t.Parallel()

		store := testkit.NewStore()
		addAda(t, store)
		store.CreateSessionErr = context.DeadlineExceeded
		handler := newAuthServer(store)

		recorder := doLogin(t, handler, `{"email":"ada@example.com","password":"correct horse battery"}`)

		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
	})
}

func TestSessionEndpointReportsTheLoggedInUser(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAda(t, store)
	handler := newAuthServer(store)
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	body := decodeBody[map[string]any](t, recorder)
	if body["email"] != "ada@example.com" {
		t.Errorf("body = %v, want the session user", body)
	}
}

func TestSessionEndpointRejectsMissingOrDeadSessions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		request func(t *testing.T, handler http.Handler) *http.Request
		want    int
	}{
		"no cookie": {
			request: func(_ *testing.T, _ http.Handler) *http.Request {
				return httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
			},
			want: http.StatusUnauthorized,
		},
		"unknown token": {
			request: func(_ *testing.T, _ http.Handler) *http.Request {
				request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
				request.AddCookie(&http.Cookie{Name: testCookieName, Value: "forged"})
				return request
			},
			want: http.StatusUnauthorized,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			store := testkit.NewStore()
			addAda(t, store)
			handler := newAuthServer(store)

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, tc.request(t, handler))

			if recorder.Code != tc.want {
				t.Errorf("status = %d, want %d", recorder.Code, tc.want)
			}
		})
	}
}

func TestSessionEndpointReportsStoreFailure(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAda(t, store)
	handler := newAuthServer(store)
	cookie := loginCookie(t, handler)
	store.SessionErr = context.DeadlineExceeded

	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestLogoutDeletesTheSessionAndClearsTheCookie(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	addAda(t, store)
	handler := newAuthServer(store)
	cookie := loginCookie(t, handler)

	request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if len(store.Sessions) != 0 {
		t.Error("session survived logout")
	}
	cleared := sessionCookie(t, recorder, testCookieName)
	if cleared.MaxAge >= 0 {
		t.Errorf("cleared cookie MaxAge = %d, want negative", cleared.MaxAge)
	}

	t.Run("without a cookie", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusNoContent {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusNoContent)
		}
	})

	t.Run("store failure", func(t *testing.T) {
		store.DeleteErr = context.DeadlineExceeded
		defer func() { store.DeleteErr = nil }()
		request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
		request.AddCookie(loginCookie(t, handler))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
	})
}

func TestConfigCookieName(t *testing.T) {
	t.Parallel()

	const name = "__Host-cms_session"
	store := testkit.NewStore()
	addAda(t, store)
	handler := newConfiguredServer(authkit.Config{Store: store, CookieName: name}, store)

	login := doLogin(t, handler, `{"email":"ada@example.com","password":"correct horse battery"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d", login.Code, http.StatusOK)
	}
	cookie := sessionCookie(t, login, name)

	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Errorf("session status = %d, want %d under the configured cookie name", recorder.Code, http.StatusOK)
	}

	logout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logout.AddCookie(cookie)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, logout)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if cleared := sessionCookie(t, recorder, name); cleared.MaxAge >= 0 {
		t.Errorf("cleared cookie MaxAge = %d, want negative", cleared.MaxAge)
	}
}

func TestConfigSessionTTL(t *testing.T) {
	t.Parallel()

	ttl := time.Hour
	store := testkit.NewStore()
	addAda(t, store)
	handler := newConfiguredServer(authkit.Config{Store: store, SessionTTL: ttl}, store)

	recorder := doLogin(t, handler, `{"email":"ada@example.com","password":"correct horse battery"}`)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	cookie := sessionCookie(t, recorder, testCookieName)
	if cookie.MaxAge != int(ttl.Seconds()) {
		t.Errorf("cookie MaxAge = %d, want %d", cookie.MaxAge, int(ttl.Seconds()))
	}
	session, ok := store.Sessions[string(gouncer.HashToken(cookie.Value))]
	if !ok {
		t.Fatal("no session persisted for the issued cookie token")
	}
	if got := session.ExpiresAt.Sub(session.CreatedAt); got != ttl {
		t.Errorf("session lifetime = %v, want %v", got, ttl)
	}
}
