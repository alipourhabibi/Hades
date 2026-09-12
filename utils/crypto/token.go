// Package crypto provides token generation and hashing utilities used
// by the session and API token subsystems.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateToken returns a prefixed, cryptographically random token together
// with its SHA-256 hex digest. prefix is prepended to the 32-byte hex entropy
// and included in the hash, so the returned hash covers the full token string.
// Pass an empty prefix for unprefixed tokens (email/password-reset links, etc.).
// Store only the hash in the database; return the token to the client.
func GenerateToken(prefix string) (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("crypto: rand.Read: %w", err)
	}
	token = prefix + hex.EncodeToString(b)
	hash = HashToken(token)
	return token, hash, nil
}

// Token type markers. The authorization interceptor routes on these, so they
// must not change without a migration for credentials already in circulation.
const (
	// APITokenPrefix marks a personal access token.
	APITokenPrefix = "hades1_"
	// SessionTokenPrefix marks an interactive session token.
	// #nosec G101 -- a token PREFIX used to route credentials by type, not a secret.
	SessionTokenPrefix = "hds_sess_"
)

// GenerateAPIToken returns a personal access token of the form
//
//	hades1_<8 hex display id>_<64 hex secret>
//
// together with the display prefix ("hades1_<8 hex display id>") and the
// SHA-256 hex digest of the full token.
//
// The display id is independent random data, never a slice of the secret, so
// storing and showing the prefix in plaintext leaks nothing about the token.
// Store only the hash; return the token to the client exactly once.
func GenerateAPIToken() (token, prefix, hash string, err error) {
	display := make([]byte, 4)
	if _, err = rand.Read(display); err != nil {
		return "", "", "", fmt.Errorf("crypto: rand.Read: %w", err)
	}
	secret := make([]byte, 32)
	if _, err = rand.Read(secret); err != nil {
		return "", "", "", fmt.Errorf("crypto: rand.Read: %w", err)
	}
	prefix = APITokenPrefix + hex.EncodeToString(display)
	token = prefix + "_" + hex.EncodeToString(secret)
	return token, prefix, HashToken(token), nil
}

// HashToken returns the SHA-256 hex digest of raw.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
