// SPDX-License-Identifier: Apache-2.0

package gouncer_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

func TestNewInvitedUserHoldsNoUsablePassword(t *testing.T) {
	t.Parallel()

	u, err := gouncer.NewInvitedUser("  Maria@Example.COM ", "  Maria Perez ")
	if err != nil {
		t.Fatalf("NewInvitedUser() error = %v, want nil", err)
	}

	if u.Email != "maria@example.com" {
		t.Errorf("email = %q, want trimmed and lowercased %q", u.Email, "maria@example.com")
	}
	if u.Name != "Maria Perez" {
		t.Errorf("name = %q, want trimmed %q", u.Name, "Maria Perez")
	}
	if u.ID == uuid.Nil {
		t.Error("id = nil, want a generated id")
	}
	if u.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at location = %v, want UTC", u.CreatedAt.Location())
	}
	if u.PasswordHash != "" {
		t.Errorf("password hash = %q, want empty until activation", u.PasswordHash)
	}
	if u.Confirmed {
		t.Error("confirmed = true, want an invited user unconfirmed")
	}
	if gouncer.VerifyPassword(u.PasswordHash, "") {
		t.Error("an empty password verifies against the empty hash, want no login before activation")
	}
	if gouncer.VerifyPassword(u.PasswordHash, "correct horse battery") {
		t.Error("a password verifies against the empty hash, want no login before activation")
	}
}

func TestNewInvitedUserRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		email   string
		name    string
		wantErr error
	}{
		"empty email":        {"", "Maria", gouncer.ErrInvalidEmail},
		"email without at":   {"maria.example.com", "Maria", gouncer.ErrInvalidEmail},
		"email with display": {"Maria <maria@example.com>", "Maria", gouncer.ErrInvalidEmail},
		"empty name":         {"maria@example.com", "   ", gouncer.ErrEmptyName},
		"name too long":      {"maria@example.com", strings.Repeat("m", 257), gouncer.ErrNameTooLong},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := gouncer.NewInvitedUser(tt.email, tt.name)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("NewInvitedUser() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewUserIsConfirmedAtConstruction(t *testing.T) {
	t.Parallel()

	u, err := gouncer.NewUser("ada@example.com", "Ada Lovelace", "correct horse battery")
	if err != nil {
		t.Fatalf("NewUser() error = %v, want nil", err)
	}

	if !u.Confirmed {
		t.Error("confirmed = false, want a password holder confirmed by construction")
	}
}

func TestSetPasswordHashesAValidPassword(t *testing.T) {
	t.Parallel()

	u, err := gouncer.NewInvitedUser("maria@example.com", "Maria Perez")
	if err != nil {
		t.Fatalf("NewInvitedUser() error = %v, want nil", err)
	}

	if err := u.SetPassword("correct horse battery"); err != nil {
		t.Fatalf("SetPassword() error = %v, want nil", err)
	}

	if !strings.HasPrefix(u.PasswordHash, "$argon2id$") {
		t.Errorf("password hash = %q, want an argon2id PHC string", u.PasswordHash)
	}
	if !gouncer.VerifyPassword(u.PasswordHash, "correct horse battery") {
		t.Error("the set password does not verify")
	}
	if u.Confirmed {
		t.Error("confirmed = true, want SetPassword to leave confirmation alone")
	}
}

func TestSetPasswordRejectsOutOfBoundsPasswords(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		password string
		wantErr  error
	}{
		"too short": {"tiny", gouncer.ErrWeakPassword},
		"too long":  {strings.Repeat("p", 1025), gouncer.ErrPasswordTooLong},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			u, err := gouncer.NewInvitedUser("maria@example.com", "Maria Perez")
			if err != nil {
				t.Fatalf("NewInvitedUser() error = %v, want nil", err)
			}

			if err := u.SetPassword(tt.password); !errors.Is(err, tt.wantErr) {
				t.Errorf("SetPassword() error = %v, want %v", err, tt.wantErr)
			}
			if u.PasswordHash != "" {
				t.Errorf("password hash = %q, want untouched by a refused password", u.PasswordHash)
			}
		})
	}
}
