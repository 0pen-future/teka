package token

import (
	"testing"
)

// TestNewUniqueness proves two mints never collide — the whole security model
// rests on the plaintext being unguessable and unique.
func TestNewUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		plaintext, hash, err := New()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if _, dup := seen[plaintext]; dup {
			t.Fatalf("New() produced a duplicate plaintext on iteration %d", i)
		}
		seen[plaintext] = struct{}{}
		if _, dup := seen[hash]; dup {
			t.Fatalf("New() produced a duplicate hash on iteration %d", i)
		}
		seen[hash] = struct{}{}
	}
}

// TestHashDeterminism proves the same plaintext always hashes to the same
// digest — lookup by hash depends on it.
func TestHashDeterminism(t *testing.T) {
	plaintext, hash, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := Hash(plaintext); got != hash {
		t.Errorf("Hash(plaintext) = %q, want %q from New()", got, hash)
	}
	if Hash("a") == Hash("b") {
		t.Error("Hash collided on distinct inputs")
	}
}

// TestHashLength pins the digest at 64 hex chars — the token_hash columns are
// VARCHAR(64).
func TestHashLength(t *testing.T) {
	if got := len(Hash("anything")); got != 64 {
		t.Errorf("len(Hash) = %d, want 64", got)
	}
}
