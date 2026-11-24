// Package textx cleans user-supplied display text.
package textx

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const zeroWidthJoiner = '‍'

// Clean normalizes to NFC, turns control characters into spaces, drops format
// characters (zero-width spaces, bidi overrides that can visually reverse text) and
// collapses whitespace. The zero-width joiner is kept because emoji sequences need it.
func Clean(s string) string {
	s = norm.NFC.String(s)
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r):
			return ' '
		case r != zeroWidthJoiner && unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}
