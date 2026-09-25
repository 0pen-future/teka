// Package likeq builds safe patterns for ILIKE/LIKE from user input.
package likeq

import "strings"

// Escape is the escape character the patterns from this package rely on.
// Queries must append `ESCAPE '\'` after the pattern placeholder.
const Escape = `\`

var escaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Contains returns a pattern that matches rows containing q literally: the
// LIKE metacharacters in q are escaped so "100%" or "a_b" do not act as
// wildcards.
func Contains(q string) string {
	return "%" + escaper.Replace(q) + "%"
}

// Prefix returns a pattern that matches rows starting with q literally.
func Prefix(q string) string {
	return escaper.Replace(q) + "%"
}
