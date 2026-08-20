// SPDX-License-Identifier: Apache-2.0

package postgres_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"
)

func TestRunCreateAdminStartsTheAccountUnderTheRankItIsGiven(t *testing.T) {
	t.Parallel()

	databaseURL := emptyDatabaseURL(t)
	var stdout strings.Builder

	err := authkitpg.RunCreateAdmin(
		t.Context(),
		databaseURL,
		[]string{"-email", "maria@example.com", "-name", "Maria Perez", "-rank", "admin"},
		strings.NewReader("correct horse battery\n"),
		&stdout,
	)
	if err != nil {
		t.Fatalf("RunCreateAdmin() error = %v, want nil", err)
	}

	store := authkitpg.NewUserStore(newPoolFor(t, databaseURL))
	held, err := store.UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want nil", err)
	}
	if held.Rank != "admin" {
		t.Errorf("rank = %q, want %q", held.Rank, "admin")
	}
}

func TestRunGrantRankReachesEveryAccountHoldingNone(t *testing.T) {
	t.Parallel()

	databaseURL := emptyDatabaseURL(t)
	pool := newPoolFor(t, databaseURL)
	if err := authkitpg.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	store := authkitpg.NewUserStore(pool)
	ada := addRanked(t, store, "ada@example.com", "")
	grace := addRanked(t, store, "grace@example.com", "editor")
	var stdout strings.Builder

	err := authkitpg.RunGrantRank(t.Context(), databaseURL, []string{"-rank", "admin"}, &stdout)
	if err != nil {
		t.Fatalf("RunGrantRank() error = %v, want nil", err)
	}

	if held, _ := store.UserByID(t.Context(), ada.ID); held.Rank != "admin" {
		t.Errorf("unranked account = %q, want %q", held.Rank, "admin")
	}
	if kept, _ := store.UserByID(t.Context(), grace.ID); kept.Rank != "editor" {
		t.Errorf("ranked account = %q, want it left at %q", kept.Rank, "editor")
	}
	if !strings.Contains(stdout.String(), "1") {
		t.Errorf("stdout = %q, want it to report how many accounts took the rank", stdout.String())
	}
}

func TestRunGrantRankRefusesAnEmptyRank(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder

	err := authkitpg.RunGrantRank(t.Context(), emptyDatabaseURL(t), []string{"-rank", ""}, &stdout)

	if !errors.Is(err, gouncer.ErrEmptyRank) {
		t.Errorf("RunGrantRank() error = %v, want ErrEmptyRank", err)
	}
}

func TestRunGrantRankRefusesWithoutADatabase(t *testing.T) {
	t.Parallel()

	var stdout strings.Builder

	err := authkitpg.RunGrantRank(t.Context(), "", []string{"-rank", "admin"}, &stdout)

	if err == nil {
		t.Error("RunGrantRank() error = nil, want a refusal naming the missing database")
	}
}

// newPoolFor returns a pool over the database the url names.
func newPoolFor(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
