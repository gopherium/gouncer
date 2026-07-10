// SPDX-License-Identifier: Apache-2.0

package gouncer_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
)

func TestNewSessionIssuesAHashedRandomToken(t *testing.T) {
	t.Parallel()

	userID := uuid.Must(uuid.NewV7())

	s, err := gouncer.NewSession(userID)
	if err != nil {
		t.Fatalf("NewSession() error = %v, want nil", err)
	}

	if s.UserID != userID {
		t.Errorf("user id = %v, want %v", s.UserID, userID)
	}
	if len(s.Token) < 40 {
		t.Errorf("token length = %d, want at least 40 characters of entropy", len(s.Token))
	}
	if !bytes.Equal(s.TokenHash, gouncer.HashToken(s.Token)) {
		t.Error("token hash does not match HashToken of the issued token")
	}
	if got := s.ExpiresAt.Sub(s.CreatedAt); got != gouncer.DefaultSessionDuration {
		t.Errorf("session lifetime = %v, want %v", got, gouncer.DefaultSessionDuration)
	}
	if s.CreatedAt.Location() != time.UTC {
		t.Errorf("created_at location = %v, want UTC", s.CreatedAt.Location())
	}
}

func TestNewSessionTokensAreUnique(t *testing.T) {
	t.Parallel()

	userID := uuid.Must(uuid.NewV7())

	first, err := gouncer.NewSession(userID)
	if err != nil {
		t.Fatalf("NewSession() error = %v, want nil", err)
	}
	second, err := gouncer.NewSession(userID)
	if err != nil {
		t.Fatalf("NewSession() error = %v, want nil", err)
	}

	if first.Token == second.Token {
		t.Error("two sessions issued the same token, want unique random tokens")
	}
}

func TestHashTokenIsDeterministicAndTokenSpecific(t *testing.T) {
	t.Parallel()

	if !bytes.Equal(gouncer.HashToken("alpha"), gouncer.HashToken("alpha")) {
		t.Error("HashToken() differs for the same token, want deterministic")
	}
	if bytes.Equal(gouncer.HashToken("alpha"), gouncer.HashToken("beta")) {
		t.Error("HashToken() equal for different tokens, want token-specific")
	}
}
