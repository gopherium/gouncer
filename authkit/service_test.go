// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"errors"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

const servicePassword = "correct horse battery"

func TestAuthenticateVerifiesCredentials(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	ada := store.AddUser(t, "ada@example.com", "Ada Lovelace", servicePassword)
	handlers := authkit.New(authkit.Config{Store: store})

	identity, err := handlers.Authenticate(t.Context(), " ADA@Example.com ", servicePassword)
	if err != nil {
		t.Fatalf("Authenticate() error = %v, want nil", err)
	}
	if identity.ID != ada.ID || identity.Email != "ada@example.com" {
		t.Errorf("identity = %+v, want ada's", identity)
	}

	_, err = handlers.Authenticate(t.Context(), "ada@example.com", "wrong")
	if !errors.Is(err, authkit.ErrInvalidCredentials) {
		t.Errorf("wrong password error = %v, want ErrInvalidCredentials", err)
	}
	_, err = handlers.Authenticate(t.Context(), "nobody@example.com", servicePassword)
	if !errors.Is(err, authkit.ErrInvalidCredentials) {
		t.Errorf("unknown email error = %v, want ErrInvalidCredentials", err)
	}
}

func TestAuthenticateRejectsDisabledAccounts(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	ada := store.AddUser(t, "ada@example.com", "Ada Lovelace", servicePassword)
	if err := store.SetUserDisabled(t.Context(), ada.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	handlers := authkit.New(authkit.Config{Store: store})

	_, err := handlers.Authenticate(t.Context(), "ada@example.com", servicePassword)
	if !errors.Is(err, authkit.ErrInvalidCredentials) {
		t.Errorf("disabled account error = %v, want ErrInvalidCredentials", err)
	}
}

func TestSessionLifecycleThroughTheSeams(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	ada := store.AddUser(t, "ada@example.com", "Ada Lovelace", servicePassword)
	handlers := authkit.New(authkit.Config{Store: store})

	cookie, err := handlers.StartSession(t.Context(), ada.ID)
	if err != nil {
		t.Fatalf("StartSession() error = %v, want nil", err)
	}
	if cookie.Name != handlers.CookieName() || cookie.Value == "" {
		t.Fatalf("cookie = %+v, want the named session cookie with a token", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.MaxAge <= 0 {
		t.Errorf("cookie flags = %+v, want HttpOnly Secure with a positive MaxAge", cookie)
	}

	identity, err := handlers.SessionIdentity(t.Context(), cookie.Value)
	if err != nil {
		t.Fatalf("SessionIdentity() error = %v, want nil", err)
	}
	if identity.ID != ada.ID {
		t.Errorf("identity = %+v, want ada's", identity)
	}

	clearing, err := handlers.EndSession(t.Context(), cookie.Value)
	if err != nil {
		t.Fatalf("EndSession() error = %v, want nil", err)
	}
	if clearing.Value != "" || clearing.MaxAge >= 0 {
		t.Errorf("clearing cookie = %+v, want an emptied expiring cookie", clearing)
	}
	if _, err := handlers.SessionIdentity(t.Context(), cookie.Value); !errors.Is(err, gouncer.ErrSessionNotFound) {
		t.Errorf("after EndSession error = %v, want ErrSessionNotFound", err)
	}

	if _, err := handlers.EndSession(t.Context(), ""); err != nil {
		t.Errorf("EndSession(no token) error = %v, want nil", err)
	}
}

func TestAccountsCarryTheirConfirmation(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "ada@example.com", "Ada Lovelace", servicePassword)
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: store})
	invites := authkit.NewInvites(authkit.InvitesConfig{Store: store})

	created, err := admin.CreateAccount(t.Context(), "maria@example.com", "Maria Perez", "password1234", "")
	if err != nil {
		t.Fatalf("CreateAccount() error = %v, want nil", err)
	}
	if !created.Confirmed {
		t.Error("a password-created account answers Confirmed false, want true")
	}
	if _, err := invites.Invite(t.Context(), "grace@example.com", "Grace Hopper", ""); err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	listed, err := admin.ListAccounts(t.Context())
	if err != nil {
		t.Fatalf("ListAccounts() error = %v, want nil", err)
	}
	confirmed := map[string]bool{}
	for _, account := range listed {
		confirmed[account.Email] = account.Confirmed
	}
	if !confirmed["maria@example.com"] {
		t.Error("the password-created account lists as unconfirmed, want confirmed")
	}
	if confirmed["grace@example.com"] {
		t.Error("the invited account lists as confirmed, want unconfirmed until activation")
	}
}

func TestAccountSeams(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	actor := store.AddUser(t, "ada@example.com", "Ada Lovelace", servicePassword)
	admin := authkit.NewAdmin(authkit.AdminConfig{Store: store})

	created, err := admin.CreateAccount(t.Context(), "maria@example.com", "Maria Perez", "password1234", "")
	if err != nil {
		t.Fatalf("CreateAccount() error = %v, want nil", err)
	}
	if created.Email != "maria@example.com" || created.Disabled {
		t.Errorf("created = %+v, want an enabled maria account", created)
	}

	listed, err := admin.ListAccounts(t.Context())
	if err != nil {
		t.Fatalf("ListAccounts() error = %v, want nil", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed %d accounts, want 2", len(listed))
	}

	if err := admin.SetAccountDisabled(t.Context(), actor.ID, created.ID, true); err != nil {
		t.Fatalf("SetAccountDisabled() error = %v, want nil", err)
	}
	if err := admin.SetAccountDisabled(t.Context(), actor.ID, actor.ID, true); !errors.Is(err, authkit.ErrSelfDisable) {
		t.Errorf("self disable error = %v, want ErrSelfDisable", err)
	}

	if _, err := admin.CreateAccount(t.Context(), "not-an-email", "Maria Perez", "password1234", ""); err == nil {
		t.Error("CreateAccount(bad email) error = nil, want a validation error")
	}
}
