// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/gopherium/gouncer/authkit"
)

// errStore is the error a failing half of a sweep answers.
var errStore = errors.New("store unreachable")

// sweepNow is the moment every sweep in these tests runs at.
var sweepNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// sweepMark is the context key the tests mark the caller's context with.
type sweepMark struct{}

// sessionsOnly is a session reaper that keeps the moment and the context mark each half was given.
type sessionsOnly struct {
	count int64
	err   error
	nows  []time.Time
	marks []any
}

// DeleteExpiredSessions keeps now and the context mark and answers count and err.
func (s *sessionsOnly) DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error) {
	s.nows = append(s.nows, now)
	s.marks = append(s.marks, ctx.Value(sweepMark{}))
	return s.count, s.err
}

// withTokens is a session reaper that sweeps tokens too.
type withTokens struct {
	sessionsOnly
	tokens   int64
	tokenErr error
}

// DeleteExpiredTokens keeps now and the context mark and answers tokens and tokenErr.
func (s *withTokens) DeleteExpiredTokens(ctx context.Context, now time.Time) (int64, error) {
	s.nows = append(s.nows, now)
	s.marks = append(s.marks, ctx.Value(sweepMark{}))
	return s.tokens, s.tokenErr
}

func TestSweepCountsSessionsAndTokens(t *testing.T) {
	t.Parallel()

	store := &withTokens{sessionsOnly: sessionsOnly{count: 3}, tokens: 2}
	ctx := context.WithValue(t.Context(), sweepMark{}, "caller")

	sessions, tokens, err := authkit.Sweep(ctx, store, sweepNow)

	if sessions != 3 || tokens != 2 || err != nil {
		t.Errorf("Sweep() = %d, %d, %v, want 3, 2, nil", sessions, tokens, err)
	}
	if want := []time.Time{sweepNow, sweepNow}; !slices.EqualFunc(store.nows, want, time.Time.Equal) {
		t.Errorf("the halves swept at %v, want both at %v", store.nows, sweepNow)
	}
	if want := []any{"caller", "caller"}; !slices.Equal(store.marks, want) {
		t.Errorf("the halves ran under %v, want the caller's context in both", store.marks)
	}
}

func TestSweepLeavesTokensToAStoreThatKeepsNone(t *testing.T) {
	t.Parallel()

	store := &sessionsOnly{count: 3}

	sessions, tokens, err := authkit.Sweep(t.Context(), store, sweepNow)

	if sessions != 3 || tokens != 0 || err != nil || len(store.nows) != 1 {
		t.Errorf("Sweep() = %d, %d, %v after %d halves, want 3, 0, nil after 1", sessions, tokens, err,
			len(store.nows))
	}
}

func TestSweepMarksAFailedSessionHalf(t *testing.T) {
	t.Parallel()

	store := &withTokens{sessionsOnly: sessionsOnly{count: 3, err: errStore}, tokens: 2}

	sessions, tokens, err := authkit.Sweep(t.Context(), store, sweepNow)

	if sessions != 0 || tokens != 0 || len(store.nows) != 1 {
		t.Errorf("Sweep() counted %d and %d after %d halves, want 0 and 0 after the session half alone",
			sessions, tokens, len(store.nows))
	}
	if !errors.Is(err, authkit.ErrSweepSessions) || !errors.Is(err, errStore) || errors.Is(err, authkit.ErrSweepTokens) {
		t.Errorf("Sweep() error = %v, want ErrSweepSessions wrapping the store's error", err)
	}
}

func TestSweepKeepsTheSessionCountWhenTheTokenHalfFails(t *testing.T) {
	t.Parallel()

	store := &withTokens{sessionsOnly: sessionsOnly{count: 3}, tokens: 2, tokenErr: errStore}

	sessions, tokens, err := authkit.Sweep(t.Context(), store, sweepNow)

	if sessions != 3 || tokens != 0 {
		t.Errorf("Sweep() counted %d and %d, want the 3 sessions and no tokens", sessions, tokens)
	}
	if !errors.Is(err, authkit.ErrSweepTokens) || !errors.Is(err, errStore) || errors.Is(err, authkit.ErrSweepSessions) {
		t.Errorf("Sweep() error = %v, want ErrSweepTokens wrapping the store's error", err)
	}
}
