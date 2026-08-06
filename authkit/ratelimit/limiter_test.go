// SPDX-License-Identifier: Apache-2.0

package ratelimit_test

import (
	"testing"
	"time"

	"github.com/gopherium/gouncer/authkit/ratelimit"
)

func TestLimiterBlocksAKeyAfterTheBudget(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewLimiter(ratelimit.Config{Limit: 3, Window: time.Minute})
	overLimitKey := "203.0.113.7"

	for attempt := range 3 {
		allowed, _, err := limiter.Check(overLimitKey)
		if err != nil {
			t.Fatalf("Check() error = %v, want nil", err)
		}
		if !allowed {
			t.Fatalf("attempt %d blocked, want the first 3 allowed", attempt+1)
		}
		if err := limiter.RecordFailure(overLimitKey); err != nil {
			t.Fatalf("RecordFailure() error = %v, want nil", err)
		}
	}

	allowed, retryAfter, err := limiter.Check(overLimitKey)
	if err != nil {
		t.Fatalf("Check() error = %v, want nil", err)
	}
	if allowed {
		t.Error("fourth attempt allowed, want it blocked")
	}
	if want := 2 * time.Minute; retryAfter != want {
		t.Errorf("retryAfter = %v, want the middleware's %v", retryAfter, want)
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewLimiter(ratelimit.Config{Limit: 1, Window: time.Minute})
	blockedKey, freshKey := "203.0.113.7", "198.51.100.9"

	if err := limiter.RecordFailure(blockedKey); err != nil {
		t.Fatalf("RecordFailure() error = %v, want nil", err)
	}

	if allowed, _, _ := limiter.Check(blockedKey); allowed {
		t.Error("blocked key allowed, want it over budget")
	}
	if allowed, _, _ := limiter.Check(freshKey); !allowed {
		t.Error("fresh key blocked, want it allowed")
	}
}

func TestLimiterDefaultsMatchTheMiddleware(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewLimiter(ratelimit.Config{})
	defaultBudgetKey := "203.0.113.7"

	for range ratelimit.DefaultLimit - 1 {
		if err := limiter.RecordFailure(defaultBudgetKey); err != nil {
			t.Fatalf("RecordFailure() error = %v, want nil", err)
		}
	}
	if allowed, _, _ := limiter.Check(defaultBudgetKey); !allowed {
		t.Error("attempt under the default budget blocked, want it allowed")
	}

	if err := limiter.RecordFailure(defaultBudgetKey); err != nil {
		t.Fatalf("RecordFailure() error = %v, want nil", err)
	}
	allowed, retryAfter, _ := limiter.Check(defaultBudgetKey)
	if allowed {
		t.Error("attempt over the default budget allowed, want it blocked")
	}
	if want := 2 * ratelimit.DefaultWindow; retryAfter != want {
		t.Errorf("retryAfter = %v, want %v", retryAfter, want)
	}
}
