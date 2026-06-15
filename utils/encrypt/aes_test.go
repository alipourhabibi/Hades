package encrypt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validKey is a 32-byte (256-bit) hex key for testing.
const validKey = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

// wrongKey is a different valid-length key.
const wrongKey = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	cases := []string{
		"hello world",
		"",
		strings.Repeat("x", 10000),
		"unicode: 日本語テスト",
	}
	for _, plaintext := range cases {
		ciphertext, err := Encrypt(validKey, plaintext)
		require.NoError(t, err)

		got, err := Decrypt(validKey, ciphertext)
		require.NoError(t, err)
		assert.Equal(t, plaintext, got)
	}
}

func TestEncrypt_Randomness(t *testing.T) {
	// Two encryptions of the same plaintext must produce different ciphertexts
	// because of the random nonce.
	c1, err := Encrypt(validKey, "same")
	require.NoError(t, err)
	c2, err := Encrypt(validKey, "same")
	require.NoError(t, err)
	assert.NotEqual(t, c1, c2)
}

func TestDecrypt_WrongKey(t *testing.T) {
	ciphertext, err := Encrypt(validKey, "secret")
	require.NoError(t, err)

	_, err = Decrypt(wrongKey, ciphertext)
	assert.Error(t, err)
}

func TestDecrypt_CorruptCiphertext(t *testing.T) {
	ciphertext, err := Encrypt(validKey, "secret")
	require.NoError(t, err)

	// Flip the last byte.
	b := []byte(ciphertext)
	b[len(b)-1] ^= 0xff
	_, err = Decrypt(validKey, string(b))
	assert.Error(t, err)
}

func TestEncrypt_InvalidKey_TooShort(t *testing.T) {
	_, err := Encrypt("aabbcc", "data")
	assert.Error(t, err)
}

func TestEncrypt_InvalidKey_NotHex(t *testing.T) {
	_, err := Encrypt(strings.Repeat("zz", 32), "data")
	assert.Error(t, err)
}

func TestDecrypt_TooShort(t *testing.T) {
	// A valid hex string but shorter than the AES-GCM nonce (12 bytes = 24 hex chars).
	_, err := Decrypt(validKey, "aabb")
	assert.Error(t, err)
}
