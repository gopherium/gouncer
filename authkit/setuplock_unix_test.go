// SPDX-License-Identifier: Apache-2.0

//go:build (unix && !aix && !solaris) || illumos

package authkit_test

import (
	"os"
	"testing"
)

// TestARenewalUnderAStrictUmaskLeavesTheLockReopenable checks a lock file made while the umask cleared the write bit.
func TestARenewalUnderAStrictUmaskLeavesTheLockReopenable(t *testing.T) {
	if asUmaskChild(t) {
		return
	}
	needsFileModes(t)
	t.Parallel()

	path := codePath(t)
	if _, err := newSetup(path).Prepare(t.Context(), nil); err != nil {
		t.Fatalf("Prepare() error = %v, want nil", err)
	}
	expire(t, path)
	test := "TestARenewalUnderAStrictUmaskLeavesTheLockReopenable"
	if out, err := underUmask(t, test, "200", path); err != nil {
		t.Fatalf("the child renewing under umask 0200 failed: %v\n%s", err, out)
	}
	expire(t, path)

	_, err := newSetup(path).Prepare(t.Context(), nil)

	if err != nil {
		t.Errorf("Prepare() renewing after a start under umask 0200 error = %v, want nil", err)
	}
	info, err := os.Lstat(path + ".lock")
	if err != nil || info.Mode() != 0o600 {
		t.Errorf("the lock file is %v, %v, want a regular file at exactly 0600", info, err)
	}
}
