// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestEnsureAdminCreatesTheAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()

	created, err := authkit.EnsureAdmin(t.Context(), store, " Admin@Example.com ", "Admin", "password1234", "admin")

	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}
	if !created {
		t.Error("EnsureAdmin() created = false, want true")
	}
	stored, err := store.UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the created user", err)
	}
	if !gouncer.VerifyPassword(stored.PasswordHash, "password1234") {
		t.Error("stored password hash does not verify against the given password")
	}
}

func TestEnsureAdminKeepsAnExistingAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	_, err := authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "first password", "admin")
	if err != nil {
		t.Fatalf("first EnsureAdmin() error = %v, want nil", err)
	}

	created, err := authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "second password", "admin")

	if err != nil {
		t.Fatalf("second EnsureAdmin() error = %v, want nil", err)
	}
	if created {
		t.Error("second EnsureAdmin() created = true, want false")
	}
	stored, err := store.UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the existing user", err)
	}
	if !gouncer.VerifyPassword(stored.PasswordHash, "first password") {
		t.Error("existing password hash changed, want it untouched")
	}
}

func TestEnsureAdminRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		email    string
		password string
	}{
		"weak password": {email: "admin@example.com", password: "short"},
		"invalid email": {email: "not an email", password: "password1234"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			created, err := authkit.EnsureAdmin(t.Context(), testkit.NewStore(), tc.email, "Admin", tc.password, "admin")

			if err == nil {
				t.Fatal("EnsureAdmin() error = nil, want a validation failure")
			}
			if created {
				t.Error("EnsureAdmin() created = true, want false")
			}
		})
	}
}

// rolelessAccount stores the admin account holding no role and returns its password hash.
func rolelessAccount(t *testing.T, store *testkit.Store) string {
	t.Helper()
	u, err := gouncer.NewUser("admin@example.com", "Maria Perez", "password1234")
	if err != nil {
		t.Fatalf("NewUser() error = %v, want nil", err)
	}
	if err := store.CreateUser(t.Context(), u); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return u.PasswordHash
}

func TestEnsureAdminStampsARolelessAccount(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	hash := rolelessAccount(t, store)

	created, err := authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "another password", "admin")

	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}
	if created {
		t.Error("EnsureAdmin() created = true, want the existing account kept")
	}
	stored, err := store.UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the stamped user", err)
	}
	if stored.Role != "admin" {
		t.Errorf("Role = %q, want the empty role stamped with admin", stored.Role)
	}
	if !gouncer.VerifyPassword(hash, "password1234") || stored.PasswordHash != hash {
		t.Error("password hash changed, want the stamp to touch only the role")
	}
}

func TestEnsureAdminLeavesADifferentRoleStanding(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	_, err := authkit.EnsureAdmin(t.Context(), store, "editor@example.com", "Editor", "password1234", "editor")
	if err != nil {
		t.Fatalf("seeding the editor: %v", err)
	}

	_, err = authkit.EnsureAdmin(t.Context(), store, "editor@example.com", "Editor", "password1234", "admin")

	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}
	stored, err := store.UserByEmail(t.Context(), "editor@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the existing user", err)
	}
	if stored.Role != "editor" {
		t.Errorf("Role = %q, want the held role left standing", stored.Role)
	}
}

func TestEnsureAdminWritesNothingForAMatchingRole(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	_, err := authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "password1234", "admin")
	if err != nil {
		t.Fatalf("seeding the admin: %v", err)
	}
	store.SetRoleErr = errors.New("a role write happened")

	_, err = authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "password1234", "admin")

	if err != nil {
		t.Errorf("EnsureAdmin() error = %v, want no role write for a role already held", err)
	}
}

func TestEnsureAdminReportsARoleItCannotStamp(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	rolelessAccount(t, store)
	store.SetRoleErr = errors.New("role store down")

	_, err := authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "password1234", "admin")

	if !errors.Is(err, store.SetRoleErr) {
		t.Errorf("EnsureAdmin() error = %v, want the failing stamp reported", err)
	}
}

// unreadableStore is a role writing store whose account reads fail.
type unreadableStore struct {
	*testkit.Store
	readErr error
}

// UserByEmail reports the failure.
func (s unreadableStore) UserByEmail(context.Context, string) (gouncer.User, error) {
	return gouncer.User{}, s.readErr
}

func TestEnsureAdminReportsAnAccountItCannotReadBack(t *testing.T) {
	t.Parallel()

	inner := testkit.NewStore()
	rolelessAccount(t, inner)
	down := errors.New("read store down")

	_, err := authkit.EnsureAdmin(
		t.Context(),
		unreadableStore{Store: inner, readErr: down},
		"admin@example.com", "Admin", "password1234", "admin",
	)

	if !errors.Is(err, down) {
		t.Errorf("EnsureAdmin() error = %v, want the failing read reported", err)
	}
}

// coreOnlyStore narrows a testkit store to the plain store surface, holding no role writes.
type coreOnlyStore struct {
	inner *testkit.Store
}

// CreateUser stores the account through the narrowed store.
func (s coreOnlyStore) CreateUser(ctx context.Context, u gouncer.User) error {
	return s.inner.CreateUser(ctx, u)
}

// UserByEmail answers through the narrowed store.
func (s coreOnlyStore) UserByEmail(ctx context.Context, email string) (gouncer.User, error) {
	return s.inner.UserByEmail(ctx, email)
}

// CreateSession stores the session through the narrowed store.
func (s coreOnlyStore) CreateSession(ctx context.Context, held gouncer.Session) error {
	return s.inner.CreateSession(ctx, held)
}

// UserBySession answers through the narrowed store.
func (s coreOnlyStore) UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (gouncer.User, error) {
	return s.inner.UserBySession(ctx, tokenHash, now)
}

// DeleteSession forgets the session through the narrowed store.
func (s coreOnlyStore) DeleteSession(ctx context.Context, tokenHash []byte) error {
	return s.inner.DeleteSession(ctx, tokenHash)
}

func TestEnsureAdminKeepsItsShapeOnAStoreWithoutRoleWrites(t *testing.T) {
	t.Parallel()

	inner := testkit.NewStore()
	rolelessAccount(t, inner)

	created, err := authkit.EnsureAdmin(
		t.Context(), coreOnlyStore{inner: inner}, "admin@example.com", "Admin", "password1234", "admin",
	)

	if err != nil {
		t.Fatalf("EnsureAdmin() error = %v, want nil", err)
	}
	if created {
		t.Error("EnsureAdmin() created = true, want false")
	}
	stored, err := inner.UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the existing user", err)
	}
	if stored.Role != "" {
		t.Errorf("Role = %q, want a store without role writes left as it was", stored.Role)
	}
}

func TestEnsureAdminReportsStoreFailure(t *testing.T) {
	t.Parallel()

	store := testkit.NewStore()
	store.CreateUserErr = errors.New("store down")

	created, err := authkit.EnsureAdmin(t.Context(), store, "admin@example.com", "Admin", "password1234", "admin")

	if !errors.Is(err, store.CreateUserErr) {
		t.Fatalf("EnsureAdmin() error = %v, want the store failure", err)
	}
	if created {
		t.Error("EnsureAdmin() created = true, want false")
	}
}
