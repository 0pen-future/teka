package idset

import (
	"testing"

	"github.com/google/uuid"
)

func TestSame(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	cases := []struct {
		name      string
		want, got []uuid.UUID
		same      bool
	}{
		{"both empty", nil, []uuid.UUID{}, true},
		{"same order", []uuid.UUID{a, b}, []uuid.UUID{a, b}, true},
		{"different order", []uuid.UUID{a, b, c}, []uuid.UUID{c, a, b}, true},
		{"missing one", []uuid.UUID{a, b}, []uuid.UUID{a}, false},
		{"extra one", []uuid.UUID{a}, []uuid.UUID{a, b}, false},
		{"duplicate hides a missing id", []uuid.UUID{a, b}, []uuid.UUID{a, a}, false},
		{"foreign id", []uuid.UUID{a, b}, []uuid.UUID{a, c}, false},
	}
	for _, tc := range cases {
		if got := Same(tc.want, tc.got); got != tc.same {
			t.Errorf("%s: Same = %v, want %v", tc.name, got, tc.same)
		}
	}
}
