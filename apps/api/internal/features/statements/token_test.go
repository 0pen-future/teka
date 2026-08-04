package statements

import (
	"regexp"
	"testing"

	"github.com/google/uuid"
)

var tokenShape = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func TestDeriveTokenIsDeterministic(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	id := uuid.Must(uuid.NewV7())

	if got, want := deriveToken(key, id), deriveToken(key, id); got != want {
		t.Errorf("deriveToken(key, id) = %q, want repeatable %q", got, want)
	}
}

func TestDeriveTokenDiffersByStatementID(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	a := uuid.Must(uuid.NewV7())
	b := uuid.Must(uuid.NewV7())

	if deriveToken(key, a) == deriveToken(key, b) {
		t.Error("deriveToken produced the same token for two different statement ids")
	}
}

func TestDeriveTokenDiffersByKey(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	keyA := []byte("0123456789abcdef0123456789abcdef")
	keyB := []byte("fedcba9876543210fedcba9876543210")

	if deriveToken(keyA, id) == deriveToken(keyB, id) {
		t.Error("deriveToken produced the same token under two different keys")
	}
}

func TestDeriveTokenShape(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	token := deriveToken(key, uuid.Must(uuid.NewV7()))

	if !tokenShape.MatchString(token) {
		t.Errorf("deriveToken(...) = %q, want 43 URL-safe base64 characters", token)
	}
}

func TestHashTokenLength(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	token := deriveToken(key, uuid.Must(uuid.NewV7()))

	hash := hashToken(token)
	if len(hash) != 32 {
		t.Errorf("len(hashToken(token)) = %d, want 32", len(hash))
	}
}
