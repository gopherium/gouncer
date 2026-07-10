// SPDX-License-Identifier: Apache-2.0

package gouncer

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestRandomnessFailuresAreReported(t *testing.T) {
	original := randRead
	defer func() { randRead = original }()
	randRead = func([]byte) (int, error) {
		return 0, errors.New("no entropy")
	}

	if _, err := hashPassword("correct horse battery"); err == nil {
		t.Error("hashPassword() error = nil, want a randomness failure")
	}
	if _, err := NewUser("ada@example.com", "Ada", "correct horse battery"); err == nil {
		t.Error("NewUser() error = nil, want a randomness failure")
	}
	if _, err := NewSession(uuid.Must(uuid.NewV7())); err == nil {
		t.Error("NewSession() error = nil, want a randomness failure")
	}
}
