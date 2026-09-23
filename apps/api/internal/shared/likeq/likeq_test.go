package likeq

import "testing"

func TestPatternsEscapeMetacharacters(t *testing.T) {
	cases := map[string]struct{ contains, prefix string }{
		"toan":   {`%toan%`, `toan%`},
		"100%":   {`%100\%%`, `100\%%`},
		"a_b":    {`%a\_b%`, `a\_b%`},
		`c:\dir`: {`%c:\\dir%`, `c:\\dir%`},
	}
	for in, want := range cases {
		if got := Contains(in); got != want.contains {
			t.Errorf("Contains(%q) = %q, want %q", in, got, want.contains)
		}
		if got := Prefix(in); got != want.prefix {
			t.Errorf("Prefix(%q) = %q, want %q", in, got, want.prefix)
		}
	}
}
