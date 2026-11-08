// SPDX-License-Identifier: Apache-2.0

package authkit_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"
)

var (
	_ authkit.InviteSender = (*recordingSender)(nil)
	_ authkit.LinkLimiter  = (*countingLimiter)(nil)
	_ authkit.LinkLimiter  = failingLimiter{}
	_ authkit.LinkLimiter  = refusingLimiter{}
	_ authkit.AdminStore   = promotingReads{}
)

// errRelayDown is the failure a sender answers when its relay refuses the mail.
var errRelayDown = errors.New("relay down")

// sentInvite is one invitation a sender was handed.
type sentInvite struct {
	to, name, link string
}

// recordingSender records every invitation it is handed, failing each one while err is set.
type recordingSender struct {
	mu   sync.Mutex
	sent []sentInvite
	err  error
}

// SendInvite records the invitation and answers the sender's failure.
func (s *recordingSender) SendInvite(_ context.Context, to, name, link string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, sentInvite{to: to, name: name, link: link})
	return s.err
}

// invites returns every invitation the sender was handed, none for a missing sender.
func (s *recordingSender) invites() []sentInvite {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sentInvite{}, s.sent...)
}

// countingLimiter refuses a key once it counted limit failures for it.
type countingLimiter struct {
	mu       sync.Mutex
	limit    int
	failures map[string]int
}

// newCountingLimiter returns a limiter refusing a key after limit failures.
func newCountingLimiter(limit int) *countingLimiter {
	return &countingLimiter{limit: limit, failures: map[string]int{}}
}

// Check reports whether key holds fewer failures than the limit, and a minute's wait when it does not.
func (l *countingLimiter) Check(key string) (bool, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failures[key] >= l.limit {
		return false, time.Minute, nil
	}
	return true, 0, nil
}

// RecordFailure counts one failure for key.
func (l *countingLimiter) RecordFailure(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key]++
	return nil
}

// count returns the failures counted for key.
func (l *countingLimiter) count(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.failures[key]
}

// failingLimiter answers its errors from Check and RecordFailure, admitting every key.
type failingLimiter struct {
	checkErr, recordErr error
}

// Check admits every key, answering the limiter's check failure.
func (l failingLimiter) Check(string) (bool, time.Duration, error) {
	return true, 0, l.checkErr
}

// RecordFailure answers the limiter's record failure.
func (l failingLimiter) RecordFailure(string) error {
	return l.recordErr
}

// refusingLimiter refuses every key for its wait.
type refusingLimiter struct {
	wait time.Duration
}

// Check refuses every key for the limiter's wait.
func (l refusingLimiter) Check(string) (bool, time.Duration, error) {
	return false, l.wait, nil
}

// RecordFailure counts nothing.
func (refusingLimiter) RecordFailure(string) error {
	return nil
}

// promotingReads gives every account it reads the admin role, as an administrator acting between two reads.
type promotingReads struct {
	*testkit.Store
}

// UserByEmail returns the account as read, then stores it holding the admin role.
func (s promotingReads) UserByEmail(ctx context.Context, email string) (gouncer.User, error) {
	read, err := s.Store.UserByEmail(ctx, email)
	if err == nil {
		promoted := read
		promoted.Role = "admin"
		s.Users[read.ID] = promoted
	}
	return read, err
}
