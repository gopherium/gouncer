// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/postgres"
	"github.com/gopherium/gouncer/authkit/postgres/testdb"
	"github.com/gopherium/gouncer/authkit/storetest"
)

// insertToken stores a token row as it stands.
const insertToken = "INSERT INTO auth.tokens (token_hash, user_id, purpose, created_at, expires_at) " +
	"VALUES ($1, $2, $3, $4, $5)"

// heldAccount is an open transaction that holds one account row for update.
type heldAccount struct {
	// tx is the transaction that holds the row.
	tx pgx.Tx
}

// Renew deletes the token expired and stores renewed inside the transaction.
func (h heldAccount) Renew(ctx context.Context, expired, renewed gouncer.Token) error {
	if _, err := h.tx.Exec(ctx, "DELETE FROM auth.tokens WHERE token_hash = $1", expired.TokenHash); err != nil {
		return err
	}
	_, err := h.tx.Exec(ctx, insertToken, renewed.TokenHash, renewed.UserID, string(renewed.Purpose),
		renewed.CreatedAt, renewed.ExpiresAt)
	return err
}

// Commit ends the hold and keeps what it wrote.
func (h heldAccount) Commit(ctx context.Context) error { return h.tx.Commit(ctx) }

// Rollback ends the hold and drops what it wrote.
func (h heldAccount) Rollback(ctx context.Context) error { return h.tx.Rollback(ctx) }

// fixtureOn returns the contract fixture of a store over pool.
func fixtureOn(pool *pgxpool.Pool) storetest.Fixture {
	return storetest.Fixture{
		Store: postgres.NewUserStore(pool),
		Plant: func(ctx context.Context, token gouncer.Token) error {
			_, err := pool.Exec(ctx, insertToken, token.TokenHash, token.UserID, string(token.Purpose),
				token.CreatedAt, token.ExpiresAt)
			return err
		},
		Hold: func(ctx context.Context, id uuid.UUID) (storetest.Held, error) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, "SELECT id FROM auth.users WHERE id = $1 FOR UPDATE", id); err != nil {
				_ = tx.Rollback(ctx)
				return nil, err
			}
			return heldAccount{tx: tx}, nil
		},
	}
}

// freshFixture returns the contract fixture of a store over a fresh test database.
func freshFixture(t *testing.T) storetest.Fixture {
	return fixtureOn(newTestPool(t))
}

const (
	// cLocale is the locale clause of a database whose collation is C.
	cLocale = "LOCALE_PROVIDER libc LOCALE 'C'"
	// turkishLocale is the locale clause of a database whose collation is the ICU Turkish one.
	turkishLocale = "LOCALE_PROVIDER icu ICU_LOCALE 'tr-TR'"
)

// collatedName returns a random database name for the collation tests.
func collatedName() string {
	return "authkit_collate_" + strings.ToLower(rand.Text())
}

// serverExec runs statement on the test server through admin.
func serverExec(t *testing.T, admin *pgxpool.Pool, statement string) {
	t.Helper()
	if _, err := admin.Exec(context.Background(), statement); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

// poolAt returns a pool on the database cfg names, closed when t ends.
func poolAt(t *testing.T, cfg pgtestdb.Config) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), cfg.URL())
	if err != nil {
		t.Fatalf("connecting to %s: %v", cfg.Database, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// serverPool returns a pool on the test server, closed when t ends.
func serverPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	return poolAt(t, testdb.Config())
}

// createdDatabase returns the settings of a database admin creates from template with options, dropped when t ends.
func createdDatabase(t *testing.T, admin *pgxpool.Pool, template, options string) pgtestdb.Config {
	t.Helper()
	cfg := testdb.Config()
	cfg.Database = collatedName()
	name, from := pgx.Identifier{cfg.Database}.Sanitize(), pgx.Identifier{template}.Sanitize()
	serverExec(t, admin, "CREATE DATABASE "+name+" TEMPLATE "+from+" "+options)
	t.Cleanup(func() { serverExec(t, admin, "DROP DATABASE "+name+" WITH (FORCE)") })
	return cfg
}

// migratedDatabase returns the settings of a migrated database admin creates under locale, dropped when t ends.
func migratedDatabase(t *testing.T, admin *pgxpool.Pool, locale string) pgtestdb.Config {
	t.Helper()
	cfg := createdDatabase(t, admin, "template0", "ENCODING 'UTF8' "+locale)
	if err := postgres.Migrate(t.Context(), cfg.URL()); err != nil {
		t.Fatalf("Migrate() under %s error = %v, want nil", locale, err)
	}
	return cfg
}

// collatedFixtures returns a builder of fixtures over fresh copies of a migrated database created under locale.
func collatedFixtures(t *testing.T, locale string) func(t *testing.T) storetest.Fixture {
	t.Helper()
	admin := serverPool(t)
	template := migratedDatabase(t, admin, locale)
	return func(t *testing.T) storetest.Fixture {
		return fixtureOn(poolAt(t, createdDatabase(t, admin, template.Database, "")))
	}
}

// collatedPool returns a pool on a migrated database created under locale, dropped when t ends.
func collatedPool(locale string) func(t *testing.T) *pgxpool.Pool {
	return func(t *testing.T) *pgxpool.Pool {
		return poolAt(t, migratedDatabase(t, serverPool(t), locale))
	}
}

// twinOf returns an account with id at email named name.
func twinOf(t *testing.T, id, email, name string) gouncer.User {
	t.Helper()
	user := namedUser(t, email, name)
	user.ID = uuid.MustParse(id)
	return user
}

func TestUserStoreKeepsTheStoreContract(t *testing.T) {
	t.Parallel()

	storetest.Run(t, freshFixture)
}

func TestUserStoreKeepsTheHeldContract(t *testing.T) {
	t.Parallel()

	storetest.RunHeld(t, freshFixture)
}

func TestUserStoreKeepsTheStoreContractUnderTheCCollation(t *testing.T) {
	t.Parallel()

	storetest.Run(t, collatedFixtures(t, cLocale))
}

func TestUserStoreListsNamesThatDifferOnlyInCaseByID(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		collation string
		pool      func(t *testing.T) *pgxpool.Pool
	}{
		{"the server default", newTestPool},
		{"C", collatedPool(cLocale)},
		{"Turkish", collatedPool(turkishLocale)},
	} {
		t.Run(tc.collation, func(t *testing.T) {
			t.Parallel()

			store := postgres.NewUserStore(tc.pool(t))
			alpha, bravo := namedUser(t, "alpha@example.com", "alpha account"), namedUser(t, "bravo@example.com",
				"Bravo account")
			lower := twinOf(t, "019a0000-0000-7000-8000-000000000001", "lower@example.com", "ivory account")
			upper := twinOf(t, "019a0000-0000-7000-8000-000000000002", "upper@example.com", "Ivory account")
			for _, u := range []gouncer.User{upper, bravo, lower, alpha} {
				if err := store.CreateUser(t.Context(), u); err != nil {
					t.Fatalf("CreateUser() error = %v, want nil", err)
				}
			}

			got, err := store.ListUsers(t.Context())

			if err != nil {
				t.Fatalf("ListUsers() error = %v, want nil", err)
			}
			names := make([]string, len(got))
			for i, u := range got {
				names[i] = u.Name + " " + u.ID.String()
			}
			want := []string{alpha.Name + " " + alpha.ID.String(), bravo.Name + " " + bravo.ID.String(),
				lower.Name + " " + lower.ID.String(), upper.Name + " " + upper.ID.String()}
			if diff := cmp.Diff(want, names); diff != "" {
				t.Errorf("ListUsers() under %s mismatch (-want +got):\n%s", tc.collation, diff)
			}
		})
	}
}
