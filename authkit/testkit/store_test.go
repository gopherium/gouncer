// SPDX-License-Identifier: Apache-2.0

package testkit_test

import (
	"testing"

	"github.com/gopherium/gouncer/authkit/storetest"
	"github.com/gopherium/gouncer/authkit/testkit"
)

func TestTheStoreKeepsTheContract(t *testing.T) {
	t.Parallel()

	storetest.Run(t, func(*testing.T) storetest.Fixture {
		return storetest.Fixture{Store: testkit.NewStore()}
	})
}
