// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

// defaultCookieName applies the __Host- prefix for Secure, Path=/,
// host-scoped session cookies.
const defaultCookieName = "__Host-session"

// Config parameterizes the session transport.
type Config struct {
	// Store persists users and their login sessions.
	Store gouncer.Store
	// CookieName names the session cookie. Empty applies "__Host-session".
	// Names should keep the __Host- prefix to retain its browser guarantees.
	CookieName string
	// SessionTTL bounds issued sessions and their cookie alike. Zero
	// applies gouncer.DefaultSessionDuration.
	SessionTTL time.Duration
}

// Handlers serves login sessions over HTTP.
type Handlers struct {
	store      gouncer.Store
	cookieName string
	ttl        time.Duration
	// newSession issues login sessions; a field so failure paths stay
	// testable.
	newSession func(userID uuid.UUID) (gouncer.Session, error)
}

// New returns Handlers serving sessions from cfg.Store.
func New(cfg Config) *Handlers {
	cookieName := cfg.CookieName
	if cookieName == "" {
		cookieName = defaultCookieName
	}
	ttl := cfg.SessionTTL
	if ttl == 0 {
		ttl = gouncer.DefaultSessionDuration
	}
	return &Handlers{
		store:      cfg.Store,
		cookieName: cookieName,
		ttl:        ttl,
		newSession: gouncer.NewSession,
	}
}

// dummyPasswordHash is verified against when a login names an unknown
// user, so both outcomes cost one hash computation. It hashes a password
// too short to register, so no account can ever share it.
const dummyPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$c3Ra23u60gssamS7GUMIlA$" +
	"gw1m1IBIwi/ojF3wkAm3P07a5LSQwos4waXky7aLVWM"

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login verifies credentials and issues a session cookie.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	body, err := Decode[credentials](w, r)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "malformed login request")
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	u, err := h.store.UserByEmail(r.Context(), email)
	if errors.Is(err, gouncer.ErrUserNotFound) {
		gouncer.VerifyPassword(dummyPasswordHash, body.Password)
		RespondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !gouncer.VerifyPassword(u.PasswordHash, body.Password) || u.Disabled {
		RespondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	session, err := h.newSession(u.ID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	session.ExpiresAt = session.CreatedAt.Add(h.ttl)
	if err := h.store.CreateSession(r.Context(), session); err != nil {
		RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}
	http.SetCookie(w, sessionCookie(h.cookieName, session.Token, int(h.ttl.Seconds())))
	Respond(w, http.StatusOK, Identity{ID: u.ID, Email: u.Email, Name: u.Name})
}

// Logout deletes the current session and clears its cookie.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(h.cookieName); err == nil {
		if err := h.store.DeleteSession(r.Context(), gouncer.HashToken(cookie.Value)); err != nil {
			RespondError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	http.SetCookie(w, sessionCookie(h.cookieName, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

// Session reports the logged-in user, whose identity the RequireSession
// middleware already resolved.
func (h *Handlers) Session(w http.ResponseWriter, r *http.Request) {
	Respond(w, http.StatusOK, IdentityFromContext(r.Context()))
}

// sessionUser returns the user owning the request's session cookie.
func (h *Handlers) sessionUser(r *http.Request) (gouncer.User, error) {
	cookie, err := r.Cookie(h.cookieName)
	if err != nil {
		return gouncer.User{}, gouncer.ErrSessionNotFound
	}
	return h.store.UserBySession(r.Context(), gouncer.HashToken(cookie.Value), time.Now().UTC())
}

// RequireSession admits only requests carrying a usable session cookie,
// passing the authenticated identity down through the request context.
func (h *Handlers) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := h.sessionUser(r)
		if errors.Is(err, gouncer.ErrSessionNotFound) {
			RespondError(w, http.StatusUnauthorized, "no session")
			return
		}
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "internal error")
			return
		}
		identity := Identity{ID: u.ID, Email: u.Email, Name: u.Name}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), identity)))
	})
}

// sessionCookie builds the named session cookie carrying token for maxAge
// seconds; a negative maxAge clears it.
func sessionCookie(name, token string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}
