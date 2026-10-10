// SPDX-License-Identifier: Apache-2.0

package testkit

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/storetest"
)

// storeHold holds the whole store under its own lock, so every method waits until the hold ends.
type storeHold struct {
	// store is the held store.
	store *Store
}

// Renew swaps renewed in for expired while the store is held.
func (h *storeHold) Renew(_ context.Context, expired, renewed gouncer.Token) error {
	delete(h.store.Tokens, string(expired.TokenHash))
	h.store.Tokens[string(renewed.TokenHash)] = renewed
	return nil
}

// Commit ends the hold.
func (h *storeHold) Commit(context.Context) error {
	h.store.mu.Unlock()
	return nil
}

// Rollback ends the hold.
func (h *storeHold) Rollback(context.Context) error {
	h.store.mu.Unlock()
	return nil
}

func TestTheStoreKeepsTheHeldContract(t *testing.T) {
	t.Parallel()

	storetest.RunHeld(t, func(*testing.T) storetest.Fixture {
		store := NewStore()
		return storetest.Fixture{
			Store: store,
			Hold: func(context.Context, uuid.UUID) (storetest.Held, error) {
				store.mu.Lock()
				return &storeHold{store: store}, nil
			},
		}
	})
}
