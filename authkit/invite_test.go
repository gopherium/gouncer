// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"errors"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

// invites returns an invite service over a fresh store beside login handlers sharing it.
func invites(cfg authkit.InvitesConfig) (*authkit.Invites, *testkit.Store, *authkit.Handlers) {
	store := testkit.NewStore()
	cfg.Store = store
	return authkit.NewInvites(cfg), store, authkit.New(authkit.Config{Store: store})
}

func TestInviteCreatesAnUnconfirmedAccountWithItsToken(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})

	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	if tok.Purpose != gouncer.PurposeInvite {
		t.Errorf("purpose = %q, want %q", tok.Purpose, gouncer.PurposeInvite)
	}
	if lifetime := tok.ExpiresAt.Sub(tok.CreatedAt); lifetime != authkit.DefaultInviteTTL {
		t.Errorf("lifetime = %v, want the default %v", lifetime, authkit.DefaultInviteTTL)
	}
	u, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the invited account stored", err)
	}
	if u.Confirmed {
		t.Error("confirmed = true, want the invited account unconfirmed")
	}
	if u.PasswordHash != "" {
		t.Error("password hash set, want none before activation")
	}
	if u.Role != "member" {
		t.Errorf("role = %q, want %q", u.Role, "member")
	}
	if tok.UserID != u.ID {
		t.Errorf("token user = %v, want the invited account %v", tok.UserID, u.ID)
	}
	_, err = handlers.Authenticate(t.Context(), "maria@example.com", "anything at all")
	if !errors.Is(err, authkit.ErrInvalidCredentials) {
		t.Errorf("Authenticate() error = %v, want ErrInvalidCredentials before activation", err)
	}
}

func TestInviteRefusesATakenAddress(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	store.AddUser(t, "maria@example.com", "Maria Perez", "correct horse battery")

	_, err := service.Invite(t.Context(), "maria@example.com", "Maria Again", "member")

	if !errors.Is(err, gouncer.ErrEmailTaken) {
		t.Errorf("Invite() error = %v, want ErrEmailTaken", err)
	}
}

func TestInviteValidatesTheIdentity(t *testing.T) {
	t.Parallel()

	service, _, _ := invites(authkit.InvitesConfig{})

	_, err := service.Invite(t.Context(), "not-an-address", "Maria Perez", "member")

	if !errors.Is(err, gouncer.ErrInvalidEmail) {
		t.Errorf("Invite() error = %v, want ErrInvalidEmail", err)
	}
}

func TestResendReplacesThePendingToken(t *testing.T) {
	t.Parallel()

	service, _, _ := invites(authkit.InvitesConfig{})
	first, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	second, err := service.ResendInvite(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("ResendInvite() error = %v, want nil", err)
	}

	if second.Token == first.Token {
		t.Error("resend answered the same secret, want a replacement")
	}
	_, err = service.RedeemInvite(t.Context(), first.Token, "correct horse battery")
	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("RedeemInvite(old) error = %v, want ErrTokenNotFound after the replacement", err)
	}
	if _, err := service.RedeemInvite(t.Context(), second.Token, "correct horse battery"); err != nil {
		t.Errorf("RedeemInvite(new) error = %v, want nil", err)
	}
}

func TestResendRefusesAnActivatedOrUnknownAccount(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")

	if _, err := service.ResendInvite(t.Context(), "ada@example.com"); !errors.Is(err, authkit.ErrAlreadyActivated) {
		t.Errorf("ResendInvite(activated) error = %v, want ErrAlreadyActivated", err)
	}
	if _, err := service.ResendInvite(t.Context(), "nobody@example.com"); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("ResendInvite(unknown) error = %v, want ErrUserNotFound", err)
	}
}

func TestRedeemInviteActivatesTheAccount(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})
	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	userID, err := service.RedeemInvite(t.Context(), tok.Token, "correct horse battery")
	if err != nil {
		t.Fatalf("RedeemInvite() error = %v, want nil", err)
	}

	if userID != tok.UserID {
		t.Errorf("user = %v, want the invited account %v", userID, tok.UserID)
	}
	u, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if !u.Confirmed {
		t.Error("confirmed = false, want redemption to confirm the address")
	}
	if _, err := handlers.Authenticate(t.Context(), "maria@example.com", "correct horse battery"); err != nil {
		t.Errorf("Authenticate() error = %v, want the activated account to log in", err)
	}
}

func TestRedeemInviteIsSingleUse(t *testing.T) {
	t.Parallel()

	service, _, _ := invites(authkit.InvitesConfig{})
	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	if _, err := service.RedeemInvite(t.Context(), tok.Token, "correct horse battery"); err != nil {
		t.Fatalf("RedeemInvite() error = %v, want nil", err)
	}

	_, err = service.RedeemInvite(t.Context(), tok.Token, "another good password")

	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("second RedeemInvite() error = %v, want ErrTokenNotFound", err)
	}
}

func TestRedeemInviteRefusesAnExpiredToken(t *testing.T) {
	t.Parallel()

	service, _, _ := invites(authkit.InvitesConfig{InviteTTL: time.Nanosecond})
	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	_, err = service.RedeemInvite(t.Context(), tok.Token, "correct horse battery")

	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("RedeemInvite(expired) error = %v, want ErrTokenNotFound", err)
	}
}

func TestRedeemInviteLeavesTheTokenWhenThePasswordIsRefused(t *testing.T) {
	t.Parallel()

	service, _, _ := invites(authkit.InvitesConfig{})
	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	if _, err := service.RedeemInvite(t.Context(), tok.Token, "tiny"); !errors.Is(err, gouncer.ErrWeakPassword) {
		t.Fatalf("RedeemInvite(weak) error = %v, want ErrWeakPassword", err)
	}

	if _, err := service.RedeemInvite(t.Context(), tok.Token, "correct horse battery"); err != nil {
		t.Errorf("RedeemInvite(retry) error = %v, want the token still redeemable", err)
	}
}

func TestRequestResetIssuesATokenForAConfirmedAccount(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	u := store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")

	tok, err := service.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}

	if tok.Purpose != gouncer.PurposeReset {
		t.Errorf("purpose = %q, want %q", tok.Purpose, gouncer.PurposeReset)
	}
	if tok.UserID != u.ID {
		t.Errorf("token user = %v, want %v", tok.UserID, u.ID)
	}
	if lifetime := tok.ExpiresAt.Sub(tok.CreatedAt); lifetime != authkit.DefaultResetTTL {
		t.Errorf("lifetime = %v, want the default %v", lifetime, authkit.DefaultResetTTL)
	}
}

func TestRequestResetAnswersNotFoundOutsideConfirmedEnabledAccounts(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	disabled := store.AddUser(t, "off@example.com", "Turned Off", "correct horse battery")
	if err := store.SetUserDisabled(t.Context(), disabled.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	if _, err := service.Invite(t.Context(), "pending@example.com", "Still Pending", "member"); err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	for name, email := range map[string]string{
		"unknown":     "nobody@example.com",
		"disabled":    "off@example.com",
		"unconfirmed": "pending@example.com",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := service.RequestReset(t.Context(), email); !errors.Is(err, gouncer.ErrUserNotFound) {
				t.Errorf("RequestReset(%s) error = %v, want ErrUserNotFound", email, err)
			}
		})
	}
}

func TestRequestResetHoldsWhileATokenStands(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	tok, err := service.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}

	if _, err := service.RequestReset(t.Context(), "ada@example.com"); !errors.Is(err, gouncer.ErrTokenExists) {
		t.Errorf("second RequestReset() error = %v, want ErrTokenExists while one stands", err)
	}

	if _, err := service.RedeemReset(t.Context(), tok.Token, "entirely new password"); err != nil {
		t.Fatalf("RedeemReset() error = %v, want nil", err)
	}
	if _, err := service.RequestReset(t.Context(), "ada@example.com"); err != nil {
		t.Errorf("RequestReset() after redemption error = %v, want a fresh token allowed", err)
	}
}

func TestRedeemResetReplacesThePasswordAndEndsEverySession(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})
	u := store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	laptop, err := handlers.StartSession(t.Context(), u.ID)
	if err != nil {
		t.Fatalf("StartSession() error = %v, want nil", err)
	}
	phone, err := handlers.StartSession(t.Context(), u.ID)
	if err != nil {
		t.Fatalf("StartSession() error = %v, want nil", err)
	}
	tok, err := service.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}

	userID, err := service.RedeemReset(t.Context(), tok.Token, "entirely new password")
	if err != nil {
		t.Fatalf("RedeemReset() error = %v, want nil", err)
	}

	if userID != u.ID {
		t.Errorf("user = %v, want %v", userID, u.ID)
	}
	if _, err := handlers.Authenticate(t.Context(), "ada@example.com", "entirely new password"); err != nil {
		t.Errorf("Authenticate(new) error = %v, want the new password accepted", err)
	}
	_, err = handlers.Authenticate(t.Context(), "ada@example.com", "correct horse battery")
	if !errors.Is(err, authkit.ErrInvalidCredentials) {
		t.Errorf("Authenticate(old) error = %v, want the old password refused", err)
	}
	for name, cookie := range map[string]string{"laptop": laptop.Value, "phone": phone.Value} {
		if _, err := handlers.SessionIdentity(t.Context(), cookie); !errors.Is(err, gouncer.ErrSessionNotFound) {
			t.Errorf("SessionIdentity(%s) error = %v, want every session ended by the reset", name, err)
		}
	}
}

func TestRedeemResetIsSingleUse(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	tok, err := service.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}
	if _, err := service.RedeemReset(t.Context(), tok.Token, "entirely new password"); err != nil {
		t.Fatalf("RedeemReset() error = %v, want nil", err)
	}

	_, err = service.RedeemReset(t.Context(), tok.Token, "yet another password")

	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("second RedeemReset() error = %v, want ErrTokenNotFound", err)
	}
}

func TestRedeemResetLeavesTheTokenWhenThePasswordIsRefused(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	tok, err := service.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}

	if _, err := service.RedeemReset(t.Context(), tok.Token, "tiny"); !errors.Is(err, gouncer.ErrWeakPassword) {
		t.Fatalf("RedeemReset(weak) error = %v, want ErrWeakPassword", err)
	}

	if _, err := service.RedeemReset(t.Context(), tok.Token, "entirely new password"); err != nil {
		t.Errorf("RedeemReset(retry) error = %v, want the token still redeemable", err)
	}
}

func TestTheStoreNeverReceivesATokenSecret(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	if _, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member"); err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	if _, err := service.RequestReset(t.Context(), "ada@example.com"); err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}

	if len(store.Tokens) != 2 {
		t.Fatalf("stored tokens = %d, want 2", len(store.Tokens))
	}
	for _, tok := range store.Tokens {
		if tok.Token != "" {
			t.Errorf("a %s token reached the store carrying its secret, want the hash alone", tok.Purpose)
		}
		if len(tok.TokenHash) == 0 {
			t.Errorf("a %s token reached the store with no hash", tok.Purpose)
		}
	}
}

func TestRedeemInviteRefusesAnActivatedAccount(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})
	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	if _, err := service.RedeemInvite(t.Context(), tok.Token, "correct horse battery"); err != nil {
		t.Fatalf("RedeemInvite() error = %v, want nil", err)
	}
	stale, err := gouncer.NewToken(tok.UserID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("NewToken() error = %v, want nil", err)
	}
	if err := store.CreateToken(t.Context(), stale); err != nil {
		t.Fatalf("CreateToken() error = %v, want nil", err)
	}

	_, err = service.RedeemInvite(t.Context(), stale.Token, "attacker chosen password")
	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("RedeemInvite() error = %v, want gouncer.ErrUserNotFound for an activated account", err)
	}

	if _, err := handlers.Authenticate(t.Context(), "maria@example.com", "attacker chosen password"); err == nil {
		t.Error("a second activation overwrote the password of an activated account")
	}
	if _, err := handlers.Authenticate(t.Context(), "maria@example.com", "correct horse battery"); err != nil {
		t.Errorf("Authenticate() with the owner's password error = %v, want it untouched", err)
	}
}

func TestDisablingAnAccountRevokesItsInvite(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})
	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	if err := store.SetUserDisabled(t.Context(), tok.UserID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	if err := store.SetUserDisabled(t.Context(), tok.UserID, false); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}

	_, err = service.RedeemInvite(t.Context(), tok.Token, "attacker chosen password")
	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("RedeemInvite() error = %v, want the disable to have revoked the token", err)
	}

	if _, err := handlers.Authenticate(t.Context(), "maria@example.com", "attacker chosen password"); err == nil {
		t.Error("a token issued before the disable set a password after the re-enable")
	}
}

func TestDisablingAnAccountRevokesItsReset(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})
	u := store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	tok, err := service.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}
	if err := store.SetUserDisabled(t.Context(), u.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	if err := store.SetUserDisabled(t.Context(), u.ID, false); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}

	_, err = service.RedeemReset(t.Context(), tok.Token, "attacker chosen password")
	if !errors.Is(err, gouncer.ErrTokenNotFound) {
		t.Errorf("RedeemReset() error = %v, want the disable to have revoked the token", err)
	}

	if _, err := handlers.Authenticate(t.Context(), "ada@example.com", "attacker chosen password"); err == nil {
		t.Error("a token issued before the disable replaced the password after the re-enable")
	}
	if _, err := handlers.Authenticate(t.Context(), "ada@example.com", "correct horse battery"); err != nil {
		t.Errorf("Authenticate() with the original password error = %v, want it untouched", err)
	}
}

func TestRedeemInviteRefusesADisabledAccount(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})
	if _, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member"); err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	invited, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if err := store.SetUserDisabled(t.Context(), invited.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	tok, err := gouncer.NewToken(invited.ID, gouncer.PurposeInvite, time.Hour)
	if err != nil {
		t.Fatalf("NewToken() error = %v, want nil", err)
	}
	store.Tokens[string(tok.TokenHash)] = tok

	_, err = service.RedeemInvite(t.Context(), tok.Token, "attacker chosen password")
	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("RedeemInvite() error = %v, want gouncer.ErrUserNotFound", err)
	}

	held, err := store.UserByID(t.Context(), invited.ID)
	if err != nil {
		t.Fatalf("UserByID() error = %v, want nil", err)
	}
	if held.Confirmed {
		t.Error("the refused redemption confirmed a disabled account, want the address left recoverable")
	}
	if held.PasswordHash != "" {
		t.Error("the refused redemption stored a password on a disabled account")
	}
	if err := store.SetUserDisabled(t.Context(), invited.ID, false); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	if _, err := handlers.Authenticate(t.Context(), "maria@example.com", "attacker chosen password"); err == nil {
		t.Error("re-enabling admitted the password the refused redemption offered")
	}
}

func TestRedeemResetRefusesADisabledAccount(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})
	u := store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	if _, err := service.RequestReset(t.Context(), "ada@example.com"); err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}
	if err := store.SetUserDisabled(t.Context(), u.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	tok, err := gouncer.NewToken(u.ID, gouncer.PurposeReset, time.Hour)
	if err != nil {
		t.Fatalf("NewToken() error = %v, want nil", err)
	}
	store.Tokens[string(tok.TokenHash)] = tok

	_, err = service.RedeemReset(t.Context(), tok.Token, "attacker chosen password")
	if !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("RedeemReset() error = %v, want gouncer.ErrUserNotFound", err)
	}

	if err := store.SetUserDisabled(t.Context(), u.ID, false); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	if _, err := handlers.Authenticate(t.Context(), "ada@example.com", "attacker chosen password"); err == nil {
		t.Error("re-enabling admitted the password the refused reset offered")
	}
	if _, err := handlers.Authenticate(t.Context(), "ada@example.com", "correct horse battery"); err != nil {
		t.Errorf("Authenticate() with the original password error = %v, want it untouched", err)
	}
}

func TestResendInviteRefusesADisabledAccount(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	if _, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member"); err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	invited, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if err := store.SetUserDisabled(t.Context(), invited.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}

	if _, err := service.ResendInvite(t.Context(), "maria@example.com"); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("ResendInvite() error = %v, want gouncer.ErrUserNotFound", err)
	}
}

func TestAFailedResetLeavesTheAccountExactlyAsItWas(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})
	u := store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	held, err := handlers.StartSession(t.Context(), u.ID)
	if err != nil {
		t.Fatalf("StartSession() error = %v, want nil", err)
	}
	tok, err := service.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}
	store.ResetErr = errors.New("users table gone")

	if _, err := service.RedeemReset(t.Context(), tok.Token, "entirely new password"); err == nil {
		t.Fatal("RedeemReset() error = nil, want the store failure surfaced")
	}

	if _, err := handlers.Authenticate(t.Context(), "ada@example.com", "correct horse battery"); err != nil {
		t.Errorf("Authenticate() with the old password error = %v, want the failed reset to have changed nothing", err)
	}
	if _, err := handlers.Authenticate(t.Context(), "ada@example.com", "entirely new password"); err == nil {
		t.Error("Authenticate() with the new password error = nil, want a failed reset to store no password")
	}
	if _, err := handlers.SessionIdentity(t.Context(), held.Value); err != nil {
		t.Errorf("SessionIdentity() error = %v, want a failed reset to end no session", err)
	}
}

func TestAFailedResetKeepsTheStandingLink(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	tok, err := service.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}
	store.ResetErr = errors.New("users table gone")
	if _, err := service.RedeemReset(t.Context(), tok.Token, "entirely new password"); err == nil {
		t.Fatal("RedeemReset() error = nil, want the store failure surfaced")
	}
	store.ResetErr = nil

	if _, err := service.RequestReset(t.Context(), "ada@example.com"); !errors.Is(err, gouncer.ErrTokenExists) {
		t.Errorf("RequestReset() error = %v, want the standing link held", err)
	}
	if _, err := service.RedeemReset(t.Context(), tok.Token, "entirely new password"); err != nil {
		t.Errorf("RedeemReset() with the standing link error = %v, want it still good", err)
	}
}

func TestAFailedInviteLeavesTheAddressResendable(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	store.TokenErr = errors.New("token table gone")

	if _, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member"); err == nil {
		t.Fatal("Invite() error = nil, want the token failure surfaced")
	}

	store.TokenErr = nil
	tok, err := service.ResendInvite(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("ResendInvite() error = %v, want the orphaned invite recoverable", err)
	}
	if _, err := service.RedeemInvite(t.Context(), tok.Token, "correct horse battery"); err != nil {
		t.Errorf("RedeemInvite() error = %v, want the resent invite to activate", err)
	}
}

func TestIssuingATokenRefusesADisabledAccount(t *testing.T) {
	t.Parallel()

	_, store, _ := invites(authkit.InvitesConfig{})
	u := store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	if err := store.SetUserDisabled(t.Context(), u.ID, true); err != nil {
		t.Fatalf("SetUserDisabled() error = %v, want nil", err)
	}
	tok, err := gouncer.NewToken(u.ID, gouncer.PurposeReset, time.Hour)
	if err != nil {
		t.Fatalf("NewToken() error = %v, want nil", err)
	}

	if err := store.CreateToken(t.Context(), tok); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("CreateToken() error = %v, want gouncer.ErrUserNotFound for a disabled account", err)
	}
	if err := store.ReplaceToken(t.Context(), tok); !errors.Is(err, gouncer.ErrUserNotFound) {
		t.Errorf("ReplaceToken() error = %v, want gouncer.ErrUserNotFound for a disabled account", err)
	}
}

func TestAFailedResendKeepsTheStandingInvite(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	store.TokenErr = errors.New("token table gone")
	if _, err := service.ResendInvite(t.Context(), "maria@example.com"); err == nil {
		t.Fatal("ResendInvite() error = nil, want the store failure surfaced")
	}
	store.TokenErr = nil

	if _, err := service.RedeemInvite(t.Context(), tok.Token, "correct horse battery"); err != nil {
		t.Errorf("RedeemInvite() error = %v, want the failed resend to have kept the standing invite", err)
	}
}

func TestAFailedActivationSpendsNoInvite(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	store.ActivateErr = errors.New("users table gone")
	if _, err := service.RedeemInvite(t.Context(), tok.Token, "correct horse battery"); err == nil {
		t.Fatal("RedeemInvite() error = nil, want the store failure surfaced")
	}

	store.ActivateErr = nil
	if _, err := service.RedeemInvite(t.Context(), tok.Token, "correct horse battery"); err != nil {
		t.Errorf("RedeemInvite() with the same secret error = %v, want the invite unspent", err)
	}
}

func TestAFailedResetSpendsNoLink(t *testing.T) {
	t.Parallel()

	service, store, handlers := invites(authkit.InvitesConfig{})
	store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	tok, err := service.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}
	store.ResetErr = errors.New("users table gone")
	if _, err := service.RedeemReset(t.Context(), tok.Token, "entirely new password"); err == nil {
		t.Fatal("RedeemReset() error = nil, want the store failure surfaced")
	}

	store.ResetErr = nil
	if _, err := service.RedeemReset(t.Context(), tok.Token, "entirely new password"); err != nil {
		t.Errorf("RedeemReset() with the same secret error = %v, want the link unspent", err)
	}
	if _, err := handlers.Authenticate(t.Context(), "ada@example.com", "entirely new password"); err != nil {
		t.Errorf("Authenticate() error = %v, want the retried reset to have landed", err)
	}
}

func TestAFailedActivationLeavesTheInviteResendable(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	tok, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}
	store.ActivateErr = errors.New("users table gone")

	if _, err := service.RedeemInvite(t.Context(), tok.Token, "correct horse battery"); err == nil {
		t.Fatal("RedeemInvite() error = nil, want the store failure surfaced")
	}

	store.ActivateErr = nil
	fresh, err := service.ResendInvite(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("ResendInvite() error = %v, want the consumed invite recoverable", err)
	}
	if _, err := service.RedeemInvite(t.Context(), fresh.Token, "correct horse battery"); err != nil {
		t.Errorf("RedeemInvite() error = %v, want the resent invite to activate", err)
	}
}

func TestAnExpiredInviteFreesItsAddress(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{InviteTTL: time.Nanosecond})
	if _, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member"); err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	if _, err := store.DeleteExpiredTokens(t.Context(), time.Now().UTC()); err != nil {
		t.Fatalf("DeleteExpiredTokens() error = %v, want nil", err)
	}

	if _, err := service.Invite(t.Context(), "maria@example.com", "Maria Again", "member"); err != nil {
		t.Errorf("Invite() after the sweep error = %v, want the address free again", err)
	}
}

func TestStoreFailuresSurface(t *testing.T) {
	t.Parallel()

	boom := errors.New("store down")
	tests := map[string]func(*testkit.Store, *authkit.Invites, string) error{
		"resend cannot clear tokens": func(s *testkit.Store, i *authkit.Invites, _ string) error {
			s.TokenErr = boom
			_, err := i.ResendInvite(t.Context(), "maria@example.com")
			return err
		},
		"redeem cannot activate": func(s *testkit.Store, i *authkit.Invites, pending string) error {
			s.ActivateErr = boom
			_, err := i.RedeemInvite(t.Context(), pending, "correct horse battery")
			return err
		},
		"reset cannot write the account": func(s *testkit.Store, i *authkit.Invites, _ string) error {
			s.ResetErr = boom
			return redeemFreshReset(t, s, i)
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			service, store, _ := invites(authkit.InvitesConfig{})
			pending, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
			if err != nil {
				t.Fatalf("Invite() error = %v, want nil", err)
			}

			if err := run(store, service, pending.Token); !errors.Is(err, boom) {
				t.Errorf("error = %v, want the store failure surfaced", err)
			}
		})
	}
}

// redeemFreshReset activates an account, requests a reset and redeems it, answering the redemption error.
func redeemFreshReset(t *testing.T, s *testkit.Store, i *authkit.Invites) error {
	t.Helper()
	s.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	tok, err := i.RequestReset(t.Context(), "ada@example.com")
	if err != nil {
		return err
	}
	_, err = i.RedeemReset(t.Context(), tok.Token, "entirely new password")
	return err
}

func TestTheSweepSparesActivatedAccountsAndLiveInvites(t *testing.T) {
	t.Parallel()

	service, store, _ := invites(authkit.InvitesConfig{})
	kept := store.AddUser(t, "ada@example.com", "Ada Lovelace", "correct horse battery")
	pending, err := service.Invite(t.Context(), "maria@example.com", "Maria Perez", "member")
	if err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	if _, err := store.DeleteExpiredTokens(t.Context(), time.Now().UTC()); err != nil {
		t.Fatalf("DeleteExpiredTokens() error = %v, want nil", err)
	}

	if _, err := store.UserByID(t.Context(), kept.ID); err != nil {
		t.Errorf("UserByID(activated) error = %v, want the activated account untouched", err)
	}
	if _, err := store.UserByID(t.Context(), pending.UserID); err != nil {
		t.Errorf("UserByID(pending) error = %v, want the live invite untouched", err)
	}
	if _, err := service.RedeemInvite(t.Context(), pending.Token, "correct horse battery"); err != nil {
		t.Errorf("RedeemInvite() error = %v, want the live invite still redeemable", err)
	}
}

func TestResendResetReplacesAStandingToken(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.AddUser(t, "maria@example.com", "Maria Perez", servicePassword)
	invites := authkit.NewInvites(authkit.InvitesConfig{Store: store})

	first, err := invites.RequestReset(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("RequestReset() error = %v, want nil", err)
	}
	if _, err := invites.RequestReset(t.Context(), "maria@example.com"); !errors.Is(err, gouncer.ErrTokenExists) {
		t.Fatalf("a second request error = %v, want ErrTokenExists", err)
	}

	second, err := invites.ResendReset(t.Context(), "maria@example.com")

	if err != nil {
		t.Fatalf("ResendReset() error = %v, want nil", err)
	}
	if second.Token == "" || second.Token == first.Token {
		t.Errorf("ResendReset() answered %q, want a fresh secret replacing %q", second.Token, first.Token)
	}
	if _, err := invites.RedeemReset(t.Context(), first.Token, "brand new password"); err == nil {
		t.Error("the replaced token still redeems, want it spent")
	}
	if _, err := invites.RedeemReset(t.Context(), second.Token, "brand new password"); err != nil {
		t.Errorf("RedeemReset(fresh) error = %v, want the replacement usable", err)
	}
}

func TestResendResetAnswersUnknownAndUnconfirmedAlike(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	invites := authkit.NewInvites(authkit.InvitesConfig{Store: store})
	if _, err := invites.Invite(t.Context(), "grace@example.com", "Grace Hopper", ""); err != nil {
		t.Fatalf("Invite() error = %v, want nil", err)
	}

	for _, address := range []string{"nobody@example.com", "grace@example.com"} {
		if _, err := invites.ResendReset(t.Context(), address); !errors.Is(err, gouncer.ErrUserNotFound) {
			t.Errorf("ResendReset(%s) error = %v, want ErrUserNotFound", address, err)
		}
	}
}
