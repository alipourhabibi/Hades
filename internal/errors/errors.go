// Package errors defines the canonical error type used across all internal
// packages. PkgError carries a status code that the Connect-RPC error
// interceptor translates into the corresponding gRPC/Connect status code
// at the transport boundary.
package errors

import (
	"fmt"
)

// Code mirrors the gRPC canonical status codes so that storage and
// service layers can signal errors without importing Connect directly.
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

// New constructs a PkgError with the given message and status code.
func New(msg string, code Code) PkgError {
	return PkgError{
		Code:    code,
		Message: msg,
	}
}

// PkgError is the structured error type used by all internal packages.
// The error interceptor converts it to a Connect/gRPC status at the
// RPC boundary.
type PkgError struct {
	Code    Code
	Message string
}

func (s PkgError) Error() string {
	return fmt.Sprintf("%v - %v", s.Code, s.Message)
}

// Status returns the error itself, satisfying interfaces that expect
// a typed status accessor.
func (p PkgError) Status() PkgError {
	return p
}

// FromError wraps an arbitrary error as a PkgError. If the error is
// already a PkgError it is returned as-is; otherwise it is wrapped
// with code Unknown.
func FromError(err error) PkgError {
	if err == nil {
		return PkgError{}
	}

	if pkgErr, ok := err.(PkgError); ok {
		return pkgErr
	}

	return PkgError{
		Code:    Unknown,
		Message: err.Error(),
	}
}
