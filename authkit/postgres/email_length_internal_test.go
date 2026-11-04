// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"errors"
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/gopherium/gouncer"
)

// addressesNoAccountCanHold holds addresses past the longest an account can hold, by name.
var addressesNoAccountCanHold = map[string]string{
	"one byte past the longest":          strings.Repeat("a", 255),
	"short in letters but long in bytes": strings.Repeat("\U000000e9", 128),
	"far past the longest":               strings.Repeat("a", 1<<20) + "@example.com",
}

// wantNothingAsked fails t unless err wraps want and the expected statement never ran.
func wantNothingAsked(t *testing.T, mock pgxmock.PgxPoolIface, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("error = %v, want it to wrap %v", err, want)
	}
	if mock.ExpectationsWereMet() == nil {
		t.Error("the store asked the database")
	}
}

func TestUserStoreStoresNoAddressNoAccountCanHold(t *testing.T) {
	t.Parallel()

	for name, address := range addressesNoAccountCanHold {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mock := mockPool(t)
			u := firstAccount()
			u.Email = address
			expectFirstInsert(mock, u, false)

			err := newUserStore(mock).CreateUser(t.Context(), u)

			wantNothingAsked(t, mock, err, gouncer.ErrInvalidEmail)
		})
	}
}

func TestCreateFirstUserStoresNoAddressNoAccountCanHold(t *testing.T) {
	t.Parallel()

	for name, address := range addressesNoAccountCanHold {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mock := mockPool(t)
			u := firstAccount()
			u.Email = address
			expectFirstBegin(mock, u, false)
			expectUsersLock(mock, u, false)
			expectReadCommitted(mock, u, false)
			expectNoUsers(mock, u, false)
			mock.ExpectRollback()

			err := newUserStore(mock).CreateFirstUser(t.Context(), u)

			wantError(t, mock, "CreateFirstUser", err, gouncer.ErrInvalidEmail)
		})
	}
}

func TestUserStoreAsksNothingAboutAnAddressNoAccountCanHold(t *testing.T) {
	t.Parallel()

	for name, address := range addressesNoAccountCanHold {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			mock := mockPool(t)
			mock.ExpectQuery("FROM auth.users").WithArgs(address).WillReturnError(errors.New("asked"))

			_, err := newUserStore(mock).UserByEmail(t.Context(), address)

			wantNothingAsked(t, mock, err, gouncer.ErrUserNotFound)
		})
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
