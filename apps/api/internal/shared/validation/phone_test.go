package validation

import "testing"

func TestVNPhonePattern(t *testing.T) {
	valid := []string{
		"0301234567", "0501234567", "0701234567", "0801234567", "0901234567",
		"+84301234567", "+84501234567", "+84701234567", "+84801234567", "+84901234567",
	}
	for _, phone := range valid {
		if !vnPhonePattern.MatchString(phone) {
			t.Errorf("%q must be accepted", phone)
		}
	}

	invalid := []string{
		"",
		"0123456789",   // 1x prefix is not a mobile range
		"0201234567",   // 2x prefix is not a mobile range
		"84901234567",  // country code without +
		"+84012345678", // +84 followed by leading 0
		"090123456",    // one digit short
		"09012345678",  // one digit long
		"+1555123456",  // foreign number
		"09o１２３４５６７",   // non-ASCII digits
		"0901 234 567", // whitespace
	}
	for _, phone := range invalid {
		if vnPhonePattern.MatchString(phone) {
			t.Errorf("%q must be rejected", phone)
		}
	}
}

func TestNormalizePhone(t *testing.T) {
	cases := map[string]string{
		"0901234567":   "+84901234567",
		"+84901234567": "+84901234567",
	}
	for in, want := range cases {
		if got := NormalizePhone(in); got != want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
}
