package errors

import (
	"fmt"
)

type Code int

const (
	Canceled           Code = 1
	Unknown            Code = 2
	InvalidArgument    Code = 3
	DeadlineExceeded   Code = 4
	NotFound           Code = 5
	AlreadyExists      Code = 6
	PermissionDenied   Code = 7
	ResourceExhausted  Code = 8
	FailedPrecondition Code = 9
	Aborted            Code = 10
	OutOfRange         Code = 11
	Unimplemented      Code = 12
	Internal           Code = 13
	Unavailable        Code = 14
	DataLoss           Code = 15
	Unauthenticated    Code = 16
)

// New returns a new PkgError which is the core logics error
func New(msg string, code Code) PkgError {
	return PkgError{
		Code:    code,
		Message: msg,
	}
}

// PkgError is our core logic error
type PkgError struct {
	Code    Code
	Message string
}

func (s PkgError) Error() string {
	return fmt.Sprintf("%v - %v", s.Code, s.Message)
}

func (p PkgError) Status() PkgError {
	return p
}

func FromError(err error) PkgError {
	if err == nil {
		return PkgError{}
	}

	if pkgErr, ok := err.(PkgError); ok {
		return pkgErr
	}

	return PkgError{
		Code:    Unknown, // Default code for unknown errors
		Message: err.Error(),
	}
}
