// Package prose writes the small pieces of English that refusals and messages share.
package prose

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// List joins names as prose: "a", "a and b", "a, b and c".
func List(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// Sentence is s with a capital first letter and a full stop.
func Sentence(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:] + "."
}
