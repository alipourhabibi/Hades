// Package crypto provides token generation and hashing utilities used
// by the session and API token subsystems.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateToken returns a cryptographically random 32-byte token as a hex
// string (raw) together with its SHA-256 hex digest (hash).
// Store only the hash in the database; return the raw token to the client.
func GenerateToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("crypto: rand.Read: %w", err)
	}
	raw = hex.EncodeToString(b)
	hash = HashToken(raw)
	return raw, hash, nil
}

// HashToken returns the SHA-256 hex digest of raw.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
