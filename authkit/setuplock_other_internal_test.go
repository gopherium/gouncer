// SPDX-License-Identifier: Apache-2.0

//go:build !((unix && !aix && !solaris) || illumos)

package authkit

// lockKept reports whether a renewal leaves a lock file beside the code file.
const lockKept = false
