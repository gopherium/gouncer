// SPDX-License-Identifier: Apache-2.0

package gouncer

import "slices"

// Roles is a set of role names an application treats alike.
type Roles []string

// Holds reports whether the role is one of the set, an account holding no role never being one.
func (r Roles) Holds(role string) bool {
	return role != "" && slices.Contains(r, role)
}
