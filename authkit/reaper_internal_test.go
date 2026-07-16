// SPDX-License-Identifier: Apache-2.0

package authkit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeReaper struct {
	mu     sync.Mutex
	calls  int
	count  int64
	err    error
	called chan struct{}
}

func (f *fakeReaper) DeleteExpiredSessions(_ context.Context, _ time.Time) (int64, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.called != nil {
		select {
		case f.called <- struct{}{}:
		default:
		}
	}
	return f.count, f.err
}

func (f *fakeReaper) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type blockingReaper struct{}

func (blockingReaper) DeleteExpiredSessions(ctx context.Context, _ time.Time) (int64, error) {
	<-ctx.Done()
	return 0, ctx.Err()
}

func TestReapOnce(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		count     int64
		err       error
		cancelCtx bool
		wantLog   string
	}{
		"deletes some":         {count: 3, wantLog: "count=3"},
		"deletes none":         {count: 0, wantLog: ""},
		"reports error":        {err: errors.New("db unreachable"), wantLog: "level=ERROR"},
		"ignores cancellation": {err: context.Canceled, cancelCtx: true, wantLog: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			reaper := &fakeReaper{count: tc.count, err: tc.err}
			ctx := t.Context()
			if tc.cancelCtx {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			reapOnce(ctx, reaper, defaultReapTimeout, slog.New(slog.NewTextHandler(&buf, nil)))

			if reaper.callCount() != 1 {
				t.Errorf("reapOnce made %d sweeps, want 1", reaper.callCount())
			}
			if tc.wantLog == "" {
				if buf.Len() != 0 {
					t.Errorf("reapOnce logged %q, want no output", buf.String())
				}
			} else if !strings.Contains(buf.String(), tc.wantLog) {
				t.Errorf("reapOnce log = %q, want it to contain %q", buf.String(), tc.wantLog)
			}
		})
	}
}

func TestReapOnceAbortsABlockedSweep(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		reapOnce(t.Context(), blockingReaper{}, 10*time.Millisecond, slog.New(slog.NewTextHandler(&buf, nil)))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reapOnce never returned from a blocked sweep")
	}
	if got := buf.String(); !strings.Contains(got, "level=ERROR") {
		t.Errorf("an aborted sweep logged %q, want an ERROR entry", got)
	}
}

func TestReapOnceStaysQuietWhenShutdownInterruptsASweep(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		reapOnce(ctx, blockingReaper{}, time.Minute, slog.New(slog.NewTextHandler(&buf, nil)))
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reapOnce never returned after shutdown cancelled the sweep")
	}
	if buf.Len() != 0 {
		t.Errorf("a shutdown-cancelled sweep logged %q, want no output", buf.String())
	}
}

func TestReapExpiredSessionsSweepsUntilCancelled(t *testing.T) {
	t.Parallel()

	reaper := &fakeReaper{called: make(chan struct{}, 1)}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		reapExpiredSessions(ctx, reaper, time.Millisecond, defaultReapTimeout, slog.New(slog.DiscardHandler))
		close(done)
	}()

	<-reaper.called // the initial sweep ran
	<-reaper.called // at least one scheduled sweep ran
	cancel()
	<-done // the loop returned on cancellation

	if reaper.callCount() < 2 {
		t.Errorf("reaper made %d sweeps, want at least 2", reaper.callCount())
	}
}

func TestNewReaperAppliesDefaults(t *testing.T) {
	t.Parallel()

	r := NewReaper(&fakeReaper{}, ReaperConfig{})

	if r.interval != defaultReapInterval {
		t.Errorf("interval = %v, want %v", r.interval, defaultReapInterval)
	}
	if r.timeout != defaultReapTimeout {
		t.Errorf("timeout = %v, want %v", r.timeout, defaultReapTimeout)
	}
	if r.log == nil {
		t.Error("logger = nil, want a default logger")
	}
}

func TestReaperStartSweepsUntilStopped(t *testing.T) {
	t.Parallel()

	reaper := &fakeReaper{called: make(chan struct{}, 1)}
	r := NewReaper(reaper, ReaperConfig{
		Interval: time.Millisecond,
		Timeout:  time.Second,
		Logger:   slog.New(slog.DiscardHandler),
	})

	r.Start()
	<-reaper.called
	r.Stop()

	swept := reaper.callCount()
	if swept < 1 {
		t.Errorf("reaper made %d sweeps, want at least 1", swept)
	}
	time.Sleep(5 * time.Millisecond)
	if reaper.callCount() != swept {
		t.Error("the reaper kept sweeping after Stop returned")
	}
}

func TestReaperStopWithoutStart(t *testing.T) {
	t.Parallel()

	r := NewReaper(&fakeReaper{}, ReaperConfig{})

	r.Stop()
}
