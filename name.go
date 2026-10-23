// SPDX-License-Identifier: Apache-2.0

package gouncer

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// cleanName follows WordPress's whitespace and C0 control handling without
// removing other Unicode characters from an account name.
func cleanName(name string) string {
	if !utf8.ValidString(name) {
		return ""
	}
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case r < 0x20:
			return -1
		default:
			return r
		}
	}, name)
	cleaned = strings.Join(strings.FieldsFunc(cleaned, func(r rune) bool { return r == ' ' }), " ")
	// Preserve U+0085, a C1 control, even though Unicode classifies it as whitespace.
	return strings.TrimFunc(cleaned, func(r rune) bool { return r != '\u0085' && unicode.IsSpace(r) })
}
