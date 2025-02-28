package errors

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

func FromBcrypt(err error) error {
	switch {
	case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
		return New(err.Error(), Unauthenticated)
	case errors.Is(err, bcrypt.ErrHashTooShort):
		return New(err.Error(), InvalidArgument) // TODO is it right?
	case errors.Is(err, bcrypt.ErrPasswordTooLong):
		return New(err.Error(), InvalidArgument)
	default:
		return New("unknown error: "+err.Error(), Unknown)
	}
}
