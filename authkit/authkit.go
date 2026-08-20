// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"errors"
	"net/http"
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
	// Privileged names the ranks RequirePrivilege admits and the guard
	// keeps an enabled account holding. Empty admits every rank.
	Privileged gouncer.Ranks
}

// Handlers serves login sessions over HTTP.
type Handlers struct {
	store      gouncer.Store
	cookieName string
	ttl        time.Duration
	privileged gouncer.Ranks
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
		privileged: cfg.Privileged,
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
		respondDecodeError(w, err, "malformed login request")
		return
	}
	identity, err := h.Authenticate(r.Context(), body.Email, body.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		RespondRefusal(w, http.StatusUnauthorized, Refusal{Message: "invalid credentials", Code: "credentials_invalid"})
		return
	}
	if err != nil {
		RespondRefusal(w, http.StatusInternalServerError, Refusal{Message: "internal error", Code: "internal"})
		return
	}
	cookie, err := h.StartSession(r.Context(), identity.ID)
	if err != nil {
		RespondRefusal(w, http.StatusInternalServerError, Refusal{Message: "internal error", Code: "internal"})
		return
	}
	http.SetCookie(w, cookie)
	Respond(w, http.StatusOK, identity)
}

// Logout deletes the current session and clears its cookie.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	token := ""
	if cookie, err := r.Cookie(h.cookieName); err == nil {
		token = cookie.Value
	}
	clearing, err := h.EndSession(r.Context(), token)
	if err != nil {
		RespondRefusal(w, http.StatusInternalServerError, Refusal{Message: "internal error", Code: "internal"})
		return
	}
	http.SetCookie(w, clearing)
	w.WriteHeader(http.StatusNoContent)
}

// Session reports the logged-in user, whose identity the RequireSession
// middleware already resolved.
func (h *Handlers) Session(w http.ResponseWriter, r *http.Request) {
	Respond(w, http.StatusOK, IdentityFromContext(r.Context()))
}

// RequirePrivilege refuses a request whose identity holds no privileged rank.
func (h *Handlers) RequirePrivilege(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(h.privileged) > 0 && !h.privileged.Holds(IdentityFromContext(r.Context()).Rank) {
			RespondRefusal(w, http.StatusForbidden, Refusal{
				Message: "rank insufficient", Code: "rank_insufficient",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireSession admits only requests carrying a usable session cookie,
// passing the authenticated identity down through the request context.
func (h *Handlers) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(h.cookieName)
		if err != nil {
			RespondRefusal(w, http.StatusUnauthorized, Refusal{Message: "no session", Code: "session_absent"})
			return
		}
		identity, err := h.SessionIdentity(r.Context(), cookie.Value)
		if errors.Is(err, gouncer.ErrSessionNotFound) {
			RespondRefusal(w, http.StatusUnauthorized, Refusal{Message: "no session", Code: "session_absent"})
			return
		}
		if err != nil {
			RespondRefusal(w, http.StatusInternalServerError, Refusal{Message: "internal error", Code: "internal"})
			return
		}
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
