package bcrypt

import (
	"golang.org/x/crypto/bcrypt"
)

// CheckPasswordHash compares a plain-text password with a hashed password
func CheckPasswordHash(password, hashedPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}
