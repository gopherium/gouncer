// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"context"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

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

// collatedName returns a random database name for the C collation tests.
func collatedName() string {
	return "authkit_collate_c_" + strings.ToLower(rand.Text())
}

// serverExec runs statement on the test server through admin.
func serverExec(t *testing.T, admin *pgxpool.Pool, statement string) {
	t.Helper()
	if _, err := admin.Exec(context.Background(), statement); err != nil {
		t.Fatalf("%s: %v", statement, err)
	}
}

// cCollationFixtures returns a builder of fixtures over fresh copies of a migrated database whose collation is C.
func cCollationFixtures(t *testing.T) func(t *testing.T) storetest.Fixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	server := testdb.Config()
	admin, err := pgxpool.New(t.Context(), server.URL())
	if err != nil {
		t.Fatalf("connecting the server pool: %v", err)
	}
	t.Cleanup(admin.Close)
	template := pgx.Identifier{collatedName()}
	serverExec(t, admin, "CREATE DATABASE "+template.Sanitize()+
		" TEMPLATE template0 ENCODING 'UTF8' LOCALE_PROVIDER libc LOCALE 'C'")
	t.Cleanup(func() { serverExec(t, admin, "DROP DATABASE "+template.Sanitize()+" WITH (FORCE)") })
	server.Database = template[0]
	if err := postgres.Migrate(t.Context(), server.URL()); err != nil {
		t.Fatalf("Migrate() of the C collation template error = %v, want nil", err)
	}
	return func(t *testing.T) storetest.Fixture {
		instance := pgx.Identifier{collatedName()}
		serverExec(t, admin, "CREATE DATABASE "+instance.Sanitize()+" TEMPLATE "+template.Sanitize())
		t.Cleanup(func() { serverExec(t, admin, "DROP DATABASE "+instance.Sanitize()+" WITH (FORCE)") })
		at := server
		at.Database = instance[0]
		pool, err := pgxpool.New(t.Context(), at.URL())
		if err != nil {
			t.Fatalf("connecting a C collation pool: %v", err)
		}
		t.Cleanup(pool.Close)
		return fixtureOn(pool)
	}
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

	storetest.Run(t, cCollationFixtures(t))
}
