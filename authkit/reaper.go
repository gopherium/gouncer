// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// defaultReapInterval is how often expired sessions are swept, and
// defaultReapTimeout bounds each sweep.
const (
	defaultReapInterval = time.Hour
	defaultReapTimeout  = 30 * time.Second
)

// SessionReaper deletes sessions that have expired.
type SessionReaper interface {
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
}

// TokenReaper deletes expired tokens and the unconfirmed accounts an
// expired invite leaves behind. A store offering it is swept alongside sessions.
type TokenReaper interface {
	DeleteExpiredTokens(ctx context.Context, now time.Time) (int64, error)
}

var (
	// ErrSweepSessions marks a sweep whose session half failed.
	ErrSweepSessions = errors.New("authkit: sweep expired sessions")
	// ErrSweepTokens marks a sweep whose token half failed.
	ErrSweepTokens = errors.New("authkit: sweep expired tokens")
)

// Sweep deletes the sessions and, on a TokenReaper, the tokens that expired by now, and counts each.
func Sweep(ctx context.Context, store SessionReaper, now time.Time) (sessions, tokens int64, err error) {
	sessions, err = store.DeleteExpiredSessions(ctx, now)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: %w", ErrSweepSessions, err)
	}
	reaper, ok := store.(TokenReaper)
	if !ok {
		return sessions, 0, nil
	}
	tokens, err = reaper.DeleteExpiredTokens(ctx, now)
	if err != nil {
		return sessions, 0, fmt.Errorf("%w: %w", ErrSweepTokens, err)
	}
	return sessions, tokens, nil
}

// ReaperConfig parameterizes a Reaper.
type ReaperConfig struct {
	// Interval is how often expired sessions are swept. Zero applies one hour.
	Interval time.Duration
	// Timeout bounds each sweep. Zero applies thirty seconds.
	Timeout time.Duration
	// Logger receives sweep outcomes. Nil applies slog.Default.
	Logger *slog.Logger
}

// Reaper periodically deletes expired sessions until stopped.
type Reaper struct {
	store    SessionReaper
	interval time.Duration
	timeout  time.Duration
	log      *slog.Logger
	cancel   context.CancelFunc
	done     chan struct{}
}

// NewReaper returns a Reaper sweeping store per cfg.
func NewReaper(store SessionReaper, cfg ReaperConfig) *Reaper {
	interval := cfg.Interval
	if interval == 0 {
		interval = defaultReapInterval
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultReapTimeout
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	return &Reaper{store: store, interval: interval, timeout: timeout, log: log}
}

// Start launches the sweep loop in a goroutine. Call it once.
func (r *Reaper) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.done = make(chan struct{})
	go func() {
		reapExpiredSessions(ctx, r.store, r.interval, r.timeout, r.log)
		close(r.done)
	}()
}

// Stop cancels the sweep loop and waits for it to finish. Stopping a
// never-started Reaper is not an error.
func (r *Reaper) Stop() {
	if r.cancel == nil {
		return
	}
	r.cancel()
	<-r.done
}

// reapExpiredSessions sweeps expired sessions once, then every interval
// until ctx is cancelled, bounding each sweep to timeout.
func reapExpiredSessions(
	ctx context.Context,
	reaper SessionReaper,
	interval time.Duration,
	timeout time.Duration,
	log *slog.Logger,
) {
	reapOnce(ctx, reaper, timeout, log)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reapOnce(ctx, reaper, timeout, log)
		}
	}
}

// reapOnce deletes the currently expired sessions and tokens within
// timeout, logging the outcomes.
func reapOnce(ctx context.Context, reaper SessionReaper, timeout time.Duration, log *slog.Logger) {
	sweepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	sessions, tokens, err := Sweep(sweepCtx, reaper, time.Now().UTC())
	if sessions > 0 {
		log.Info("reaped expired sessions", "count", sessions)
	}
	if tokens > 0 {
		log.Info("reaped expired tokens", "count", tokens)
	}
	if err == nil || ctx.Err() != nil {
		return
	}
	message := "reap expired sessions"
	if errors.Is(err, ErrSweepTokens) {
		message = "reap expired tokens"
	}
	log.Error(message, "error", err)
}
