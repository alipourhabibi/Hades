package bcrypt

import (
	"golang.org/x/crypto/bcrypt"
)

// CheckPasswordHash compares a plain-text password with a hashed password
func CheckPasswordHash(password, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}
