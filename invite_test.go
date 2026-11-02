// SPDX-License-Identifier: Apache-2.0

package gouncer_test

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode"

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
		"email with C0 control": {
			"a\x05b@example.com", "Maria", gouncer.ErrInvalidEmail,
		},
		"email with C1 control U+0085": {
			"a\u0085b@example.com", "Maria", gouncer.ErrInvalidEmail,
		},
		"email with C1 control U+009B": {
			"a\u009b8mb@example.com", "Maria", gouncer.ErrInvalidEmail,
		},
		"email with an invalid UTF-8 byte": {
			"a\x9bb@example.com", "Maria", gouncer.ErrInvalidEmail,
		},
		"email with U+0130": {
			"A\U00000130@example.com", "Maria", gouncer.ErrInvalidEmail,
		},
		"email with Kelvin sign U+212A": {
			"\U0000212Aelvin@example.com", "Maria", gouncer.ErrInvalidEmail,
		},
		"empty name":    {"maria@example.com", "   ", gouncer.ErrEmptyName},
		"name too long": {"maria@example.com", strings.Repeat("m", 257), gouncer.ErrNameTooLong},
		"name of C0 controls only": {
			"maria@example.com", "\x00\x01\x1f", gouncer.ErrEmptyName,
		},
		"name with a stray byte":       {"maria@example.com", "Maria\xff", gouncer.ErrEmptyName},
		"name with an overlong NUL":    {"maria@example.com", "\xc0\x80", gouncer.ErrEmptyName},
		"name with a surrogate half":   {"maria@example.com", "\xed\xa0\x80", gouncer.ErrEmptyName},
		"name above U+10FFFF":          {"maria@example.com", "\xf4\x90\x80\x80", gouncer.ErrEmptyName},
		"name too long after cleaning": {"maria@example.com", strings.Repeat("m", 257) + "\x00", gouncer.ErrNameTooLong},
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

func TestNewInvitedUserRefusesEveryC1Control(t *testing.T) {
	t.Parallel()

	for _, email := range addressesWithC1Controls() {
		if _, err := gouncer.NewInvitedUser(email, "Maria Perez"); !errors.Is(err, gouncer.ErrInvalidEmail) {
			t.Errorf("NewInvitedUser(%+q) error = %v, want %v", email, err, gouncer.ErrInvalidEmail)
		}
	}
}

// addressesWithC1Controls returns addresses holding each C1 control in the local part, the domain or at an end.
func addressesWithC1Controls() []string {
	var emails []string
	for r := rune(0x80); r <= 0x9f; r++ {
		c := string(r)
		emails = append(emails, "ma"+c+"ria@example.com", "maria@exa"+c+"mple.com")
		if !unicode.IsSpace(r) {
			emails = append(emails, c+"maria@example.com", "maria@example.com"+c)
		}
	}
	return emails
}
