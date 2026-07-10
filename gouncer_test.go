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

func TestNewUserNormalizesIdentityAndHashesThePassword(t *testing.T) {
	t.Parallel()

	u, err := gouncer.NewUser("  Ada@Example.COM ", "  Ada Lovelace ", "correct horse battery")
	if err != nil {
		t.Fatalf("NewUser() error = %v, want nil", err)
	}

	if u.Email != "ada@example.com" {
		t.Errorf("email = %q, want trimmed and lowercased %q", u.Email, "ada@example.com")
	}
	if u.Name != "Ada Lovelace" {
		t.Errorf("name = %q, want trimmed %q", u.Name, "Ada Lovelace")
	}
	if u.ID == uuid.Nil {
		t.Error("id = nil, want a generated id")
	}
	if u.Disabled {
		t.Error("disabled = true, want new users enabled")
	}
	if u.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at location = %v, want UTC", u.CreatedAt.Location())
	}
	if !strings.HasPrefix(u.PasswordHash, "$argon2id$") {
		t.Errorf("password hash = %q, want an argon2id PHC string", u.PasswordHash)
	}
	if strings.Contains(u.PasswordHash, "correct horse battery") {
		t.Error("password hash contains the plain password")
	}
}

func TestNewUserRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		email    string
		name     string
		password string
		wantErr  error
	}{
		"empty email":        {"", "Ada", "correct horse battery", gouncer.ErrInvalidEmail},
		"email without at":   {"ada.example.com", "Ada", "correct horse battery", gouncer.ErrInvalidEmail},
		"email with display": {"Ada <ada@example.com>", "Ada", "correct horse battery", gouncer.ErrInvalidEmail},
		"email too long": {
			strings.Repeat("a", 250) + "@example.com", "Ada", "correct horse battery", gouncer.ErrInvalidEmail,
		},
		"empty name":     {"ada@example.com", "   ", "correct horse battery", gouncer.ErrEmptyName},
		"name too long":  {"ada@example.com", strings.Repeat("a", 257), "correct horse battery", gouncer.ErrNameTooLong},
		"short password": {"ada@example.com", "Ada", "eleven char", gouncer.ErrWeakPassword},
		"password too long": {
			"ada@example.com", "Ada", strings.Repeat("x", 1025), gouncer.ErrPasswordTooLong,
		},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := gouncer.NewUser(tc.email, tc.name, tc.password)

			if !errors.Is(err, tc.wantErr) {
				t.Errorf("NewUser() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestVerifyPasswordAcceptsOnlyTheOriginalPassword(t *testing.T) {
	t.Parallel()

	u, err := gouncer.NewUser("ada@example.com", "Ada", "correct horse battery")
	if err != nil {
		t.Fatalf("NewUser() error = %v, want nil", err)
	}

	if !gouncer.VerifyPassword(u.PasswordHash, "correct horse battery") {
		t.Error("VerifyPassword() = false for the original password, want true")
	}
	if gouncer.VerifyPassword(u.PasswordHash, "wrong password entirely") {
		t.Error("VerifyPassword() = true for a wrong password, want false")
	}
	if gouncer.VerifyPassword("not-a-phc-string", "correct horse battery") {
		t.Error("VerifyPassword() = true for a malformed hash, want false")
	}
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"wrong algorithm":        "$argon2i$v=19$m=19456,t=2,p=1$AAAA$AAAA",
		"unparseable version":    "$argon2id$v=x$m=19456,t=2,p=1$AAAA$AAAA",
		"wrong version":          "$argon2id$v=18$m=19456,t=2,p=1$AAAA$AAAA",
		"unparseable parameters": "$argon2id$v=19$bogus$AAAA$AAAA",
		"invalid salt encoding":  "$argon2id$v=19$m=19456,t=2,p=1$!bad!$AAAA",
		"invalid key encoding":   "$argon2id$v=19$m=19456,t=2,p=1$AAAA$!bad!",
		"empty key":              "$argon2id$v=19$m=19456,t=2,p=1$AAAA$",
		"zero time cost":         "$argon2id$v=19$m=19456,t=0,p=1$AAAA$AAAA",
		"zero threads":           "$argon2id$v=19$m=19456,t=2,p=0$AAAA$AAAA",
		"zero memory":            "$argon2id$v=19$m=0,t=2,p=1$AAAA$AAAA",
		"excessive memory":       "$argon2id$v=19$m=4294967295,t=2,p=1$AAAA$AAAA",
		"excessive time cost":    "$argon2id$v=19$m=64,t=4294967295,p=1$AAAA$AAAA",
		"excessive threads":      "$argon2id$v=19$m=64,t=2,p=255$AAAA$AAAA",
	}

	for testName, hash := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			if gouncer.VerifyPassword(hash, "correct horse battery") {
				t.Errorf("VerifyPassword(%q) = true, want false", hash)
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("no randomness")
}

func TestNewUserReportsIDGenerationFailure(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	_, err := gouncer.NewUser("ada@example.com", "Ada", "correct horse battery")

	if err == nil {
		t.Fatal("NewUser() error = nil, want an id generation failure")
	}
}

func TestPasswordHashesAreSalted(t *testing.T) {
	t.Parallel()

	first, err := gouncer.NewUser("ada@example.com", "Ada", "correct horse battery")
	if err != nil {
		t.Fatalf("NewUser() error = %v, want nil", err)
	}
	second, err := gouncer.NewUser("ada@example.com", "Ada", "correct horse battery")
	if err != nil {
		t.Fatalf("NewUser() error = %v, want nil", err)
	}

	if first.PasswordHash == second.PasswordHash {
		t.Error("two hashes of the same password are identical, want unique salts")
	}
}
