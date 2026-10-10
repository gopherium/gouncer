// SPDX-License-Identifier: Apache-2.0

package storetest

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"testing"
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
