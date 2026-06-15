package crypto

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken_Format(t *testing.T) {
	raw, hash, err := GenerateToken()
	require.NoError(t, err)

	// raw must be 64 hex chars (32 bytes)
	assert.Len(t, raw, 64)
	b, err := hex.DecodeString(raw)
	require.NoError(t, err)
	assert.Len(t, b, 32)

	// hash must be 64 hex chars (SHA-256 = 32 bytes)
	assert.Len(t, hash, 64)
	_, err = hex.DecodeString(hash)
	assert.NoError(t, err)
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	const n = 20
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		raw, _, err := GenerateToken()
		require.NoError(t, err)
		_, dup := seen[raw]
		assert.False(t, dup, "duplicate token generated")
		seen[raw] = struct{}{}
	}
}

func TestGenerateToken_HashMatchesRaw(t *testing.T) {
	raw, hash, err := GenerateToken()
	require.NoError(t, err)
	assert.Equal(t, HashToken(raw), hash)
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
