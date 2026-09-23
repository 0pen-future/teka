package classcode

import (
	"regexp"
	"testing"
)

func TestGenerateProducesValidDistinctCodes(t *testing.T) {
	shape := regexp.MustCompile(`^L[0-9A-HJKMNP-TV-Z]{6}$`)
	seen := map[string]bool{}
	for i := 0; i < 500; i++ {
		code := Generate()
		if !shape.MatchString(code) {
			t.Fatalf("code %q does not match L + 6 Crockford base32 chars", code)
		}
		if !Valid(code) {
			t.Fatalf("generated code %q must pass Valid", code)
		}
		if seen[code] {
			t.Fatalf("duplicate code %q in 500 draws", code)
		}
		seen[code] = true
	}
}

func TestGenerateNeverCollidesWithBackfillShape(t *testing.T) {
	backfill := regexp.MustCompile(`^L\d{4}$`)
	for i := 0; i < 200; i++ {
		if code := Generate(); backfill.MatchString(code) {
			t.Fatalf("code %q collides with the migration backfill shape L0001", code)
		}
	}
}

func TestValid(t *testing.T) {
	cases := map[string]bool{
		"L0001":                 true,
		"TOAN9C":                true,
		"A-1":                   true,
		"AB":                    true,
		"ABCDEFGHIJKLMNOPQRST":  true,
		"A":                     false,
		"ABCDEFGHIJKLMNOPQRSTU": false,
		"toan9c":                false,
		"TOÁN":                  false,
		"A B":                   false,
		"":                      false,
	}
	for code, want := range cases {
		if got := Valid(code); got != want {
			t.Errorf("Valid(%q) = %v, want %v", code, got, want)
		}
	}
}
