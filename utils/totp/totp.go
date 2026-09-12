// Package totp provides TOTP secret generation, code validation, and
// backup code generation for two-factor authentication.
package totp

import (
	"crypto/rand"
	"crypto/subtle"
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

// Period and Skew are the validation window. Skew 1 accepts the previous and
// next step as well as the current one, so a code is live for roughly ninety
// seconds. That is a usability allowance for clock drift, and it is exactly why
// ValidateCodeCounter exists: without recording which step was consumed, a code
// observed in transit or over someone's shoulder can be used more than once
// inside that window.
const (
	Period = 30
	Skew   = 1
)

// ValidateCode validates a 6-digit TOTP code against the given base32 secret.
//
// Prefer ValidateCodeCounter, which also reports which time step matched so the
// caller can refuse to accept the same one twice.
func ValidateCode(secret, code string) (bool, error) {
	_, valid, err := ValidateCodeCounter(secret, code, time.Now())
	return valid, err
}

// ValidateCodeCounter validates a code and returns the time step it matched.
//
// The step is the counter the RFC 6238 HOTP value was derived from
// (unix seconds / Period). A caller that records the highest step it has
// accepted for an account, and refuses anything at or below it, makes a code
// single-use rather than reusable for the length of the skew window.
//
// Steps are tried oldest first, so the returned counter is the earliest one
// that matches; that is the conservative choice, because accepting it advances
// the caller's high-water mark by the least amount.
func ValidateCodeCounter(secret, code string, now time.Time) (counter uint64, valid bool, err error) {
	// #nosec G115 -- unix seconds are positive for any clock this code can run under.
	base := uint64(now.Unix()) / Period
	for delta := -Skew; delta <= Skew; delta++ {
		// #nosec G115 -- step is bounded by unix seconds / Period and cannot be negative here.
		step := int64(base) + int64(delta)
		if step < 0 {
			continue
		}
		at := time.Unix(step*Period, 0)
		want, genErr := totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{
			Period:    Period,
			Skew:      0,
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		})
		if genErr != nil {
			return 0, false, fmt.Errorf("totp: validate: %w", genErr)
		}
		// Constant time: the comparison is against a secret-derived value.
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return uint64(step), true, nil
		}
	}
	return 0, false, nil
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
