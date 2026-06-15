// Package totp provides TOTP secret generation, code validation, and
// backup code generation for two-factor authentication.
package totp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// GenerateSecret creates a new TOTP secret for the given issuer and account.
// Returns the base32-encoded secret and the otpauth:// URL.
func GenerateSecret(issuer, accountName string) (secret, otpauthURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		Algorithm:   otp.AlgorithmSHA1,
		Digits:      otp.DigitsSix,
		Period:      30,
	})
	if err != nil {
		return "", "", fmt.Errorf("totp: generate: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// ValidateCode validates a 6-digit TOTP code against the given base32 secret.
func ValidateCode(secret, code string) (bool, error) {
	valid, err := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return false, fmt.Errorf("totp: validate: %w", err)
	}
	return valid, nil
}

// GenerateBackupCodes returns n random hex backup codes (plaintext, each 16 chars).
func GenerateBackupCodes(n int) ([]string, error) {
	codes := make([]string, n)
	for i := range codes {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("totp: backup code rand: %w", err)
		}
		codes[i] = hex.EncodeToString(b)
	}
	return codes, nil
}
