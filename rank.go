// SPDX-License-Identifier: Apache-2.0

package gouncer

import "slices"

// Ranks is a set of rank names an application treats alike.
type Ranks []string

// Holds reports whether the rank is one of the set.
func (r Ranks) Holds(rank string) bool {
	return slices.Contains(r, rank)
}
