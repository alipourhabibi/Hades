package errors

import (
	"errors"

	"gorm.io/gorm"
)

var ErrSessionExpired = errors.New("Session Expired")

// TODO may add other errors later
// FromGorm converts the gorm error to pkg error
func FromGorm(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return New(err.Error(), NotFound)
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return New(err.Error(), AlreadyExists)
	case errors.Is(err, ErrSessionExpired):
		return New(err.Error(), Unauthenticated)
	default:
		return New("unknown error: "+err.Error(), Unknown)
	}
}
