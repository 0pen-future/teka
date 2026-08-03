package id_test

import (
	"testing"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/id"
)

func TestNewReturnsVersion7(t *testing.T) {
	t.Parallel()
	u := id.New()
	if got := u.Version(); got != 7 {
		t.Fatalf("uuid version = %d, want 7", got)
	}
	if got := u.Variant(); got != uuid.RFC4122 {
		t.Fatalf("uuid variant = %v, want RFC4122", got)
	}
}

func TestNewStringParsesBackToV7(t *testing.T) {
	t.Parallel()
	s := id.NewString()
	u, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	if u.Version() != 7 {
		t.Fatalf("uuid version = %d, want 7", u.Version())
	}
}

func TestNewIsTimeOrdered(t *testing.T) {
	t.Parallel()
	// V7 ids embed a millisecond timestamp prefix, so ids generated in
	// sequence must never sort backwards.
	prev := id.New()
	for range 100 {
		next := id.New()
		if next.String() < prev.String() {
			t.Fatalf("uuidv7 sorted backwards: %s then %s", prev, next)
		}
		prev = next
	}
}
