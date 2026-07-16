// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

type failingStdin struct{}

func (failingStdin) Read([]byte) (int, error) {
	return 0, errors.New("stdin exploded")
}

func TestCreateAdminProvisionsAUser(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	var stdout strings.Builder

	err := authkit.CreateAdmin(
		t.Context(),
		store,
		" Admin@Example.com ",
		"Admin",
		strings.NewReader("correct horse battery\n"),
		&stdout,
	)

	if err != nil {
		t.Fatalf("CreateAdmin() error = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "Password: ") {
		t.Errorf("output = %q, want it to prompt for the password", stdout.String())
	}
	if !strings.Contains(stdout.String(), "admin@example.com") {
		t.Errorf("output = %q, want it to name the created user", stdout.String())
	}
	created, err := store.UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the created user", err)
	}
	if !gouncer.VerifyPassword(created.PasswordHash, "correct horse battery") {
		t.Error("stored password hash does not verify against the entered password")
	}
}

func TestCreateAdminRejectsDuplicateEmail(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()

	if err := authkit.CreateAdmin(
		t.Context(), store, "admin@example.com", "Admin", strings.NewReader("correct horse battery\n"), io.Discard,
	); err != nil {
		t.Fatalf("first CreateAdmin() error = %v, want nil", err)
	}

	err := authkit.CreateAdmin(
		t.Context(), store, "admin@example.com", "Admin", strings.NewReader("correct horse battery\n"), io.Discard,
	)

	if !errors.Is(err, gouncer.ErrEmailTaken) {
		t.Errorf("CreateAdmin() error = %v, want %v", err, gouncer.ErrEmailTaken)
	}
}

func TestCreateAdminValidatesItsInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		email string
		name  string
		stdin io.Reader
	}{
		"invalid email":     {email: "not-an-email", name: "Admin", stdin: strings.NewReader("correct horse battery\n")},
		"empty name":        {email: "admin@example.com", name: " ", stdin: strings.NewReader("correct horse battery\n")},
		"short password":    {email: "admin@example.com", name: "Admin", stdin: strings.NewReader("short\n")},
		"no password input": {email: "admin@example.com", name: "Admin", stdin: strings.NewReader("")},
		"failing stdin":     {email: "admin@example.com", name: "Admin", stdin: failingStdin{}},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			err := authkit.CreateAdmin(t.Context(), testkit.NewStore(), tc.email, tc.name, tc.stdin, io.Discard)

			if err == nil {
				t.Fatal("CreateAdmin() error = nil, want a failure")
			}
		})
	}
}
