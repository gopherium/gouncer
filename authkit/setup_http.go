// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"errors"
	"net/http"
)

// setupStatus is the body Status answers.
type setupStatus struct {
	// Open reports whether the site still waits for its first account.
	Open bool `json:"open"`
}

// codeRequest is the body CheckCode reads.
type codeRequest struct {
	// Code is the setup code as typed.
	Code string `json:"code"`
}

// Status reports over HTTP whether the site still waits for its first account, a nil store holding none.
func (s *Setup) Status(store SetupStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		err := s.refuseClosed(r.Context(), store)
		if err != nil && !errors.Is(err, ErrSetupDone) {
			respondAuthError(w, err)
			return
		}
		Respond(w, http.StatusOK, setupStatus{Open: err == nil})
	})
}

// CheckCode checks a posted setup code over HTTP and spends nothing, for a wizard's code screen.
func (s *Setup) CheckCode(store SetupStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodeSetup[codeRequest](w, r, s, store)
		if !ok {
			return
		}
		if err := s.Check(r.Context(), store, req.Code); err != nil {
			respondAuthError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// Create creates the first account over HTTP from the posted setup code and details.
func (s *Setup) Create(store SetupStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, ok := decodeSetup[FirstAccount](w, r, s, store)
		if !ok {
			return
		}
		account, err := s.CreateFirstAccount(r.Context(), store, req)
		if err != nil {
			respondAuthError(w, err)
			return
		}
		Respond(w, http.StatusCreated, account)
	})
}

// decodeSetup marks the answer uncacheable, refuses a set-up site before the body, then decodes the body as a T.
func decodeSetup[T any](w http.ResponseWriter, r *http.Request, s *Setup, store SetupStore) (T, bool) {
	var zero T
	w.Header().Set("Cache-Control", "no-store")
	if err := s.refuseClosed(r.Context(), store); err != nil {
		respondAuthError(w, err)
		return zero, false
	}
	req, err := Decode[T](w, r)
	if err != nil {
		respondDecodeError(w, err, "malformed setup request")
		return zero, false
	}
	return req, true
}
