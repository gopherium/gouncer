// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/gopherium/gouncer"
)

func TestUserStoreAsksNothingAboutAnAddressNoAccountCanHold(t *testing.T) {
	t.Parallel()

	for name, address := range map[string]string{
		"one byte past the longest":          strings.Repeat("a", 255),
		"short in letters but long in bytes": strings.Repeat("\U000000e9", 128),
		"far past the longest":               strings.Repeat("a", 1<<20) + "@example.com",
	} {
		mock := mockPool(t)
		mock.ExpectQuery("FROM auth.users").WithArgs(address).WillReturnError(errors.New("asked"))

		_, err := newUserStore(mock).UserByEmail(t.Context(), address)

		if !errors.Is(err, gouncer.ErrUserNotFound) {
			t.Errorf("%s: UserByEmail() error = %v, want %v", name, err, gouncer.ErrUserNotFound)
		}
		if mock.ExpectationsWereMet() == nil {
			t.Errorf("%s: UserByEmail() asked the database", name)
		}
	}
}

func TestUserStoreAsksAboutTheLongestAddressAnAccountCanHold(t *testing.T) {
	t.Parallel()

	mock := mockPool(t)
	longest := strings.Repeat("a", 254)
	mock.ExpectQuery("FROM auth.users").WithArgs(longest).WillReturnError(errBackend)

	_, err := newUserStore(mock).UserByEmail(t.Context(), longest)

	wantError(t, mock, "UserByEmail", err, errBackend)
}

func TestUserStoreSetUserDisabledReportsQueryFailure(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v, want nil", err)
	}
	defer mock.Close()
	id := uuid.Must(uuid.NewV7())
	backend := errors.New("backend unavailable")
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(id, true).WillReturnError(backend)
	mock.ExpectRollback()

	err = newUserStore(mock).SetUserDisabled(t.Context(), id, true)

	if !errors.Is(err, backend) {
		t.Errorf("SetUserDisabled() error = %v, want it to wrap %v", err, backend)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUserStoreSetUserDisabledReportsSessionRevokeFailure(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v, want nil", err)
	}
	defer mock.Close()
	id := uuid.Must(uuid.NewV7())
	backend := errors.New("backend unavailable")
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(id, true).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("DELETE FROM auth.sessions").WithArgs(id).WillReturnError(backend)
	mock.ExpectRollback()

	err = newUserStore(mock).SetUserDisabled(t.Context(), id, true)

	if !errors.Is(err, backend) {
		t.Errorf("SetUserDisabled() error = %v, want it to wrap %v", err, backend)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestUserStoreSetUserDisabledReportsCommitFailure(t *testing.T) {
	t.Parallel()

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v, want nil", err)
	}
	defer mock.Close()
	id := uuid.Must(uuid.NewV7())
	backend := errors.New("backend unavailable")
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE auth.users").WithArgs(id, false).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit().WillReturnError(backend)
	mock.ExpectRollback()

	err = newUserStore(mock).SetUserDisabled(t.Context(), id, false)

	if !errors.Is(err, backend) {
		t.Errorf("SetUserDisabled() error = %v, want it to wrap %v", err, backend)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
