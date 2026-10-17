// SPDX-License-Identifier: Apache-2.0

//go:build (unix && !aix && !solaris) || illumos

package authkit

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// lockSuffix names the lock file beside the code file, the code file's path with .lock added.
const lockSuffix = ".lock"

// lockMode is the mode of the lock file beside the code file.
const lockMode = 0o600

// flock applies or removes an advisory lock on the open file fd.
var flock = syscall.Flock

// lockRenewal waits for the private lock file beside the code file, refusing a link, and answers its release.
func (s *Setup) lockRenewal() (func(), error) {
	path := s.cfg.CodeFile + lockSuffix
	lock, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, lockMode)
	if err != nil {
		return nil, lockFailure(path, err)
	}
	_ = lock.Chmod(lockMode)
	if err := flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return nil, errors.Join(lockFailure(path, err), lock.Close())
	}
	return func() { _ = lock.Close() }, nil
}

// lockFailure wraps err as a failure to take the lock file at path.
func lockFailure(path string, err error) error {
	return fmt.Errorf("authkit: take the setup code lock at %s: %w", path, err)
}
