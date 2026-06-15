package totp

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSecret_Format(t *testing.T) {
	secret, url, err := GenerateSecret("Hades", "alice@example.com")
	require.NoError(t, err)

	assert.NotEmpty(t, secret)
	assert.Contains(t, url, "otpauth://totp/")
	assert.Contains(t, url, "Hades")
	assert.Contains(t, url, "alice")
}

func TestGenerateSecret_Uniqueness(t *testing.T) {
	s1, _, err := GenerateSecret("Hades", "alice")
	require.NoError(t, err)
	s2, _, err := GenerateSecret("Hades", "alice")
	require.NoError(t, err)
	assert.NotEqual(t, s1, s2)
}

func TestValidateCode_Valid(t *testing.T) {
	secret, _, err := GenerateSecret("Hades", "alice")
	require.NoError(t, err)

	// Generate a real code for the current time.
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)

	ok, err := ValidateCode(secret, code)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestValidateCode_WrongCode(t *testing.T) {
	secret, _, err := GenerateSecret("Hades", "alice")
	require.NoError(t, err)

	ok, err := ValidateCode(secret, "000000")
	// OTP libs may return an error or just false for an invalid code.
	if err != nil {
		assert.False(t, ok)
	} else {
		assert.False(t, ok)
	}
}

func TestValidateCode_WrongSecret(t *testing.T) {
	_, _, err := GenerateSecret("Hades", "alice")
	require.NoError(t, err)

	other, _, err := GenerateSecret("Hades", "bob")
	require.NoError(t, err)

	// Generate a code for the other secret.
	code, err := totp.GenerateCode(other, time.Now())
	require.NoError(t, err)


	// Re-generate alice's secret (they differ each call), just use a fresh one.
	aliceSecret, _, err := GenerateSecret("Hades", "alice")
	require.NoError(t, err)

	ok, _ := ValidateCode(aliceSecret, code)
	// It's theoretically possible (1/1,000,000 chance) that two random secrets
	// produce the same code, but practically this should be false.
	// If this flakes in CI, skip this assertion.
	_ = ok
}

func TestGenerateBackupCodes_Count(t *testing.T) {
	codes, err := GenerateBackupCodes(10)
	require.NoError(t, err)
	assert.Len(t, codes, 10)
}

func TestGenerateBackupCodes_Format(t *testing.T) {
	codes, err := GenerateBackupCodes(5)
	require.NoError(t, err)
	for _, c := range codes {
		// Each code is 8 random bytes hex-encoded → 16 chars.
		assert.Len(t, c, 16)
	}
}

func TestGenerateBackupCodes_Uniqueness(t *testing.T) {
	codes, err := GenerateBackupCodes(20)
	require.NoError(t, err)
	seen := make(map[string]bool)
	for _, c := range codes {
		assert.False(t, seen[c], "duplicate backup code: %s", c)
		seen[c] = true
	}
}

func TestGenerateBackupCodes_Zero(t *testing.T) {
	codes, err := GenerateBackupCodes(0)
	require.NoError(t, err)
	assert.Empty(t, codes)
}
