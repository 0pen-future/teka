// Package classcode owns the class code (mã lớp) contract: the shape a code
// must have and the generator every class-creating path shares — the
// service, the roster import, the seeds and the test fixtures. It imports no
// feature package so any of them can depend on it.
package classcode

import (
	"crypto/rand"
	"regexp"
)

// crockford is the base32 alphabet without I, L, O and U, so a code read
// aloud or typed from a printout is never ambiguous.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// generatedLen is the number of random characters after the "L" prefix:
// 32^6 ≈ 1e9 combinations per center, far beyond any center's class count.
const generatedLen = 6

// pattern is the wire rule for a caller-supplied code: uppercase letters,
// digits and dashes, 2–20 characters (the column is VARCHAR(20)).
var pattern = regexp.MustCompile(`^[A-Z0-9-]{2,20}$`)

// Generate returns a fresh code: "L" plus six Crockford base32 characters.
// The migration backfill numbers legacy classes "L0001"-style (four digits),
// a shape six random characters can never produce, so the two never collide.
func Generate() string {
	buf := make([]byte, generatedLen)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand only fails when the OS entropy source is broken; there
		// is nothing sensible to fall back to.
		panic("classcode: crypto/rand unavailable: " + err.Error())
	}
	out := make([]byte, 0, generatedLen+1)
	out = append(out, 'L')
	for _, b := range buf {
		out = append(out, crockford[int(b)%len(crockford)])
	}
	return string(out)
}

// Valid reports whether a caller-supplied code has the accepted shape.
func Valid(code string) bool {
	return pattern.MatchString(code)
}
