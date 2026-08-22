package secrets_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/secrets"
)

func testKey(t *testing.T, fill byte) []byte {
	t.Helper()
	return bytes.Repeat([]byte{fill}, 32)
}

func newCipher(t *testing.T, fill byte) *secrets.Cipher {
	t.Helper()
	c, err := secrets.New(testKey(t, fill))
	require.NoError(t, err)
	return c
}

func TestNewRejectsShortKey(t *testing.T) {
	t.Parallel()
	for _, size := range []int{0, 1, 16, 31} {
		_, err := secrets.New(bytes.Repeat([]byte{7}, size))
		require.ErrorIs(t, err, secrets.ErrKeyTooShort, "key of %d bytes must be rejected", size)
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	t.Parallel()
	c := newCipher(t, 0xA1)

	cases := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte{}},
		{"short", []byte("x")},
		{"credentials json", []byte(`{"imei":"abc","cookie":[{"name":"zpw_sek","value":"v"}],"userAgent":"ua"}`)},
		{"binary", []byte{0x00, 0xFF, 0x10, 0x00, 0x7F}},
		{"large", bytes.Repeat([]byte("zalo"), 4096)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sealed, err := c.Seal(tc.plaintext)
			require.NoError(t, err)
			require.NotEmpty(t, sealed)

			opened, err := c.Open(sealed)
			require.NoError(t, err)
			// Compared as strings so an empty plaintext round-tripping to an
			// empty-but-nil slice still counts as equal.
			require.Equal(t, string(tc.plaintext), string(opened))
		})
	}
}

// The ciphertext must never be a recognisable transform of the plaintext, and
// two seals of the same input must differ — a repeated nonce under one key
// breaks GCM outright.
func TestSealUsesFreshNonce(t *testing.T) {
	t.Parallel()
	c := newCipher(t, 0xB2)
	plaintext := []byte("same input every time")

	first, err := c.Seal(plaintext)
	require.NoError(t, err)
	second, err := c.Seal(plaintext)
	require.NoError(t, err)

	require.NotEqual(t, first, second, "identical ciphertexts mean the nonce was reused")
	require.NotContains(t, string(first), string(plaintext))
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	t.Parallel()
	c := newCipher(t, 0xC3)
	sealed, err := c.Seal([]byte("credentials worth stealing"))
	require.NoError(t, err)

	cases := map[string]func([]byte) []byte{
		"flipped nonce byte": func(b []byte) []byte {
			out := bytes.Clone(b)
			out[0] ^= 0xFF
			return out
		},
		"flipped body byte": func(b []byte) []byte {
			out := bytes.Clone(b)
			out[len(out)/2] ^= 0xFF
			return out
		},
		"flipped tag byte": func(b []byte) []byte {
			out := bytes.Clone(b)
			out[len(out)-1] ^= 0xFF
			return out
		},
		"truncated": func(b []byte) []byte { return bytes.Clone(b[:len(b)-1]) },
		"appended":  func(b []byte) []byte { return append(bytes.Clone(b), 0x00) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := c.Open(mutate(sealed))
			require.Error(t, err)
		})
	}
}

func TestOpenRejectsWrongKey(t *testing.T) {
	t.Parallel()
	sealed, err := newCipher(t, 0xD4).Seal([]byte("secret"))
	require.NoError(t, err)

	_, err = newCipher(t, 0xD5).Open(sealed)
	require.Error(t, err)
}

func TestOpenRejectsUndersizedInput(t *testing.T) {
	t.Parallel()
	c := newCipher(t, 0xE6)
	for _, size := range []int{0, 1, 11, 12} {
		_, err := c.Open(bytes.Repeat([]byte{9}, size))
		require.Errorf(t, err, "input of %d bytes must be rejected", size)
	}
}

// Two ciphers built from the same key bytes must be interchangeable: the KEK
// comes from configuration and is rebuilt on every process start.
func TestCiphersFromSameKeyInterop(t *testing.T) {
	t.Parallel()
	sealed, err := newCipher(t, 0xF7).Seal([]byte("survives a restart"))
	require.NoError(t, err)

	opened, err := newCipher(t, 0xF7).Open(sealed)
	require.NoError(t, err)
	require.Equal(t, []byte("survives a restart"), opened)
}
