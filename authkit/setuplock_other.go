// SPDX-License-Identifier: Apache-2.0

//go:build !((unix && !aix && !solaris) || illumos)

package authkit

// lockRenewal takes no lock on this system and answers a call that frees nothing.
func (*Setup) lockRenewal() (func(), error) {
	return func() {}, nil
}
