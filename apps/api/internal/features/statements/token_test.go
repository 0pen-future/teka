package statements

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"regexp"
	"testing"

	"github.com/google/uuid"
)

var tokenShape = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func TestDeriveTokenIsDeterministic(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	id := uuid.Must(uuid.NewV7())
	classID := uuid.Must(uuid.NewV7())

	if got, want := deriveToken(key, id, nil), deriveToken(key, id, nil); got != want {
		t.Errorf("deriveToken(key, id, nil) = %q, want repeatable %q", got, want)
	}
	if got, want := deriveToken(key, id, &classID), deriveToken(key, id, &classID); got != want {
		t.Errorf("deriveToken(key, id, class) = %q, want repeatable %q", got, want)
	}
}

func TestDeriveTokenDiffersByStatementID(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	a := uuid.Must(uuid.NewV7())
	b := uuid.Must(uuid.NewV7())

	if deriveToken(key, a, nil) == deriveToken(key, b, nil) {
		t.Error("deriveToken produced the same token for two different statement ids")
	}
}

func TestDeriveTokenDiffersByKey(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	keyA := []byte("0123456789abcdef0123456789abcdef")
	keyB := []byte("fedcba9876543210fedcba9876543210")

	if deriveToken(keyA, id, nil) == deriveToken(keyB, id, nil) {
		t.Error("deriveToken produced the same token under two different keys")
	}
}

func TestDeriveTokenDiffersByClass(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	id := uuid.Must(uuid.NewV7())
	classA := uuid.Must(uuid.NewV7())
	classB := uuid.Must(uuid.NewV7())

	family := deriveToken(key, id, nil)
	if got := deriveToken(key, id, &classA); got == family {
		t.Error("a class-scoped token must differ from the family token of the same statement id")
	}
	if deriveToken(key, id, &classA) == deriveToken(key, id, &classB) {
		t.Error("deriveToken produced the same token for two different classes")
	}
}

// A nil class must contribute nothing to the MAC: every family token minted
// before class scoping existed has to keep resolving byte-identically, or
// links already sent to parents would silently die.
func TestDeriveTokenNilClassKeepsLegacyFamilyTokens(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	id := uuid.Must(uuid.NewV7())

	mac := hmac.New(sha256.New, key)
	mac.Write(id[:])
	legacy := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if got := deriveToken(key, id, nil); got != legacy {
		t.Errorf("deriveToken(key, id, nil) = %q, want the pre-class-scoping token %q", got, legacy)
	}
}

func TestDeriveTokenShape(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	token := deriveToken(key, uuid.Must(uuid.NewV7()), nil)

	if !tokenShape.MatchString(token) {
		t.Errorf("deriveToken(...) = %q, want 43 URL-safe base64 characters", token)
	}
}

func TestHashTokenLength(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	token := deriveToken(key, uuid.Must(uuid.NewV7()), nil)

	hash := hashToken(token)
	if len(hash) != 32 {
		t.Errorf("len(hashToken(token)) = %d, want 32", len(hash))
	}
}
