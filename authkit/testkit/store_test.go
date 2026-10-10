// SPDX-License-Identifier: Apache-2.0

package testkit_test

import (
	"context"
	"testing"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit/storetest"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestTheStoreKeepsTheContract(t *testing.T) {
	t.Parallel()

	storetest.Run(t, func(*testing.T) storetest.Fixture {
		store := testkit.NewStore()
		return storetest.Fixture{
			Store: store,
			Plant: func(_ context.Context, token gouncer.Token) error {
				store.Tokens[string(token.TokenHash)] = token
				return nil
			},
		}
	})
}
