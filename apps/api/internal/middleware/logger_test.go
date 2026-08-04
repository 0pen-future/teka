package middleware

import "testing"

func TestSanitizePath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "non-statement path is untouched",
			path: "/api/v1/statements/11111111-1111-1111-1111-111111111111",
			want: "/api/v1/statements/11111111-1111-1111-1111-111111111111",
		},
		{
			name: "bare token path is redacted",
			path: "/public/statements/eW91LWNhbi1uZXZlci1zZWUtdGhpcy10b2tlbg",
			want: "/public/statements/[redacted]",
		},
		{
			name: "qr.png path keeps its suffix, redacts only the token",
			path: "/public/statements/eW91LWNhbi1uZXZlci1zZWUtdGhpcy10b2tlbg/qr.png",
			want: "/public/statements/[redacted]/qr.png",
		},
		{
			name: "prefix alone (no token segment) is redacted, not crashed on",
			path: "/public/statements/",
			want: "/public/statements/[redacted]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizePath(tc.path)
			if got != tc.want {
				t.Errorf("sanitizePath(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
