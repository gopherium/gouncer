// SPDX-License-Identifier: Apache-2.0

package gouncer_test

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

func TestNewTokenIssuesAHashedSecret(t *testing.T) {
	t.Parallel()

	userID := uuid.Must(uuid.NewV7())
	tok, err := gouncer.NewToken(userID, gouncer.PurposeInvite, 7*24*time.Hour)
	if err != nil {
		t.Fatalf("NewToken() error = %v, want nil", err)
	}

	if tok.Token == "" {
		t.Error("token = empty, want a fresh secret")
	}
	if !bytes.Equal(tok.TokenHash, gouncer.HashToken(tok.Token)) {
		t.Error("token hash does not match the hashed secret")
	}
	if tok.UserID != userID {
		t.Errorf("user id = %v, want %v", tok.UserID, userID)
	}
	if tok.Purpose != gouncer.PurposeInvite {
		t.Errorf("purpose = %q, want %q", tok.Purpose, gouncer.PurposeInvite)
	}
	if tok.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at location = %v, want UTC", tok.CreatedAt.Location())
	}
	if want := tok.CreatedAt.Add(7 * 24 * time.Hour); !tok.ExpiresAt.Equal(want) {
		t.Errorf("expires_at = %v, want created_at plus the lifetime %v", tok.ExpiresAt, want)
	}
}

func TestNewTokenSecretsAreUnique(t *testing.T) {
	t.Parallel()

	userID := uuid.Must(uuid.NewV7())
	first, err := gouncer.NewToken(userID, gouncer.PurposeReset, time.Hour)
	if err != nil {
		t.Fatalf("NewToken() error = %v, want nil", err)
	}
	second, err := gouncer.NewToken(userID, gouncer.PurposeReset, time.Hour)
	if err != nil {
		t.Fatalf("NewToken() error = %v, want nil", err)
	}

	if first.Token == second.Token {
		t.Error("two tokens share a secret, want unique secrets")
	}
}

func TestNewTokenRejectsAnEmptyPurpose(t *testing.T) {
	t.Parallel()

	_, err := gouncer.NewToken(uuid.Must(uuid.NewV7()), "", time.Hour)

	if !errors.Is(err, gouncer.ErrEmptyPurpose) {
		t.Errorf("NewToken() error = %v, want ErrEmptyPurpose", err)
	}
}

func TestNewTokenRejectsANonPositiveLifetime(t *testing.T) {
	t.Parallel()

	for name, ttl := range map[string]time.Duration{"zero": 0, "negative": -time.Hour} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := gouncer.NewToken(uuid.Must(uuid.NewV7()), gouncer.PurposeInvite, ttl)

			if !errors.Is(err, gouncer.ErrTokenLifetime) {
				t.Errorf("NewToken() error = %v, want ErrTokenLifetime", err)
			}
		})
	}
}

func TestTokenPurposesAreDistinct(t *testing.T) {
	t.Parallel()

	purposes := map[gouncer.TokenPurpose]bool{
		gouncer.PurposeInvite:  true,
		gouncer.PurposeReset:   true,
		gouncer.PurposeConfirm: true,
	}

	if len(purposes) != 3 {
		t.Errorf("purposes = %d distinct values, want 3", len(purposes))
	}
}
