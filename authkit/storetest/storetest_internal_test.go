// SPDX-License-Identifier: Apache-2.0

package storetest

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"
)

// errProbe is the error the meta-tests hand the helpers.
var errProbe = errors.New("probe failure")

// recorder is a testing.TB that keeps the failures a check reports and ends the check at a fatal one.
type recorder struct {
	testing.TB
	// failures are the messages the check reported.
	failures []string
}

// Helper marks nothing.
func (r *recorder) Helper() {}

// Errorf keeps the message.
func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

// Fatalf keeps the message and ends the check.
func (r *recorder) Fatalf(format string, args ...any) {
	r.Errorf(format, args...)
	runtime.Goexit()
}

// reported runs check under a recorder and returns what it reported.
func reported(t *testing.T, check func(t testing.TB)) []string {
	t.Helper()
	r := &recorder{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		check(r)
	}()
	<-done
	return r.failures
}

func TestExpectReportsAFalseCondition(t *testing.T) {
	t.Parallel()

	got := reported(t, func(t testing.TB) {
		expect(t, false, "the %s is wrong", "count")
		expect(t, true, "a true condition reported")
	})

	if want := []string{"the count is wrong"}; !slices.Equal(got, want) {
		t.Errorf("expect reported %q, want %q", got, want)
	}
}

func TestSettleAnswersAValueThatArrives(t *testing.T) {
	t.Parallel()

	ch := make(chan int, 1)
	ch <- 7

	if got, arrived := settle(ch, time.Second); got != 7 || !arrived {
		t.Errorf("settle() = %d, %t, want 7, true", got, arrived)
	}
}

func TestSettleGivesUpOnASilentChannel(t *testing.T) {
	t.Parallel()

	if got, arrived := settle(make(chan int), time.Millisecond); got != 0 || arrived {
		t.Errorf("settle() = %d, %t, want 0, false", got, arrived)
	}
}

func TestLaterKeepsAnAnswerThatCameEarly(t *testing.T) {
	t.Parallel()

	if got, arrived := later(make(chan int), 5, true); got != 5 || !arrived {
		t.Errorf("later() = %d, %t, want the early 5, true", got, arrived)
	}
}

func TestLaterWaitsWhenNothingCameEarly(t *testing.T) {
	t.Parallel()

	ch := make(chan int, 1)
	ch <- 9

	if got, arrived := later(ch, 0, false); got != 9 || !arrived {
		t.Errorf("later() = %d, %t, want 9, true", got, arrived)
	}
}

func TestMustEndsTheCheckAtAnError(t *testing.T) {
	t.Parallel()

	got := reported(t, func(t testing.TB) {
		must(t, nil, "CreateUser")
		must(t, errProbe, "CreateSession")
		t.Errorf("the check went on past the error")
	})

	if want := []string{"CreateSession: probe failure"}; !slices.Equal(got, want) {
		t.Errorf("must reported %q, want %q", got, want)
	}
}
