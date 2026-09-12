package crypto

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken_Format(t *testing.T) {
	token, hash, err := GenerateToken("")
	require.NoError(t, err)

	// no prefix: token must be 64 hex chars (32 bytes)
	assert.Len(t, token, 64)
	b, err := hex.DecodeString(token)
	require.NoError(t, err)
	assert.Len(t, b, 32)

	// hash must be 64 hex chars (SHA-256 = 32 bytes)
	assert.Len(t, hash, 64)
	_, err = hex.DecodeString(hash)
	assert.NoError(t, err)
}

func TestGenerateToken_WithPrefix(t *testing.T) {
	token, hash, err := GenerateToken("hds_sess_")
	require.NoError(t, err)
	assert.True(t, len(token) > 64)
	assert.Equal(t, "hds_sess_", token[:9])
	assert.Equal(t, HashToken(token), hash)
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	const n = 20
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		token, _, err := GenerateToken("")
		require.NoError(t, err)
		_, dup := seen[token]
		assert.False(t, dup, "duplicate token generated")
		seen[token] = struct{}{}
	}
}

func TestGenerateToken_HashMatchesRaw(t *testing.T) {
	token, hash, err := GenerateToken("")
	require.NoError(t, err)
	assert.Equal(t, HashToken(token), hash)
}

func TestGenerateAPIToken_Format(t *testing.T) {
	token, prefix, hash, err := GenerateAPIToken()
	require.NoError(t, err)

	// prefix: "hades1_" + 8 hex chars (4 bytes)
	assert.Equal(t, APITokenPrefix, prefix[:len(APITokenPrefix)])
	assert.Len(t, prefix, len(APITokenPrefix)+8)
	_, err = hex.DecodeString(prefix[len(APITokenPrefix):])
	assert.NoError(t, err)

	// token: prefix + "_" + 64 hex chars of secret
	assert.Equal(t, prefix+"_", token[:len(prefix)+1])
	secret := token[len(prefix)+1:]
	assert.Len(t, secret, 64)
	_, err = hex.DecodeString(secret)
	assert.NoError(t, err)

	// hash covers the full token, prefix included
	assert.Equal(t, HashToken(token), hash)
}

func TestGenerateAPIToken_PrefixIsNotPartOfSecret(t *testing.T) {
	// The display id must be independent randomness: storing it in plaintext
	// must not reveal any part of the secret.
	for i := 0; i < 20; i++ {
		token, prefix, _, err := GenerateAPIToken()
		require.NoError(t, err)
		display := prefix[len(APITokenPrefix):]
		secret := token[len(prefix)+1:]
		assert.NotEqual(t, display, secret[:len(display)])
	}
}

func TestGenerateAPIToken_Uniqueness(t *testing.T) {
	const n = 20
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		token, _, _, err := GenerateAPIToken()
		require.NoError(t, err)
		_, dup := seen[token]
		assert.False(t, dup, "duplicate API token generated")
		seen[token] = struct{}{}
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	const token = "deadbeefdeadbeefdeadbeefdeadbeef"
	h1 := HashToken(token)
	h2 := HashToken(token)
	assert.Equal(t, h1, h2)
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := HashToken("aaa")
	h2 := HashToken("bbb")
	assert.NotEqual(t, h1, h2)
}

func TestHashToken_KnownValue(t *testing.T) {
	// sha256("") = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", HashToken(""))
}
