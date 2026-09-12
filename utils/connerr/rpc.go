// Package connerr translates storage and handler errors into Connect status
// codes, and provides the constructors handlers use to raise them.
//
// The rule the package exists to enforce: a client is told the code and a
// generic message, never the internal detail. Callers log the detail
// themselves, and Internal keeps it attached to the error so a caller that
// forgets still leaves it recoverable.
package connerr

import (
	"context"
	"errors"

	"connectrpc.com/connect"
)

// ErrNotFound is the sentinel storage packages wrap when a lookup finds
// nothing, so that FromDB can translate it without importing them.
//
// Storage packages declare their own wrapped sentinel, for example
// commit.ErrNotFound, and callers match on that; FromDB matches on this one.
var ErrNotFound = errors.New("not found")

// Convenience constructors for common Connect error codes.

func NotFound(msg string) error { return connect.NewError(connect.CodeNotFound, errors.New(msg)) }
func InvalidArgument(msg string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}
func AlreadyExists(msg string) error {
	return connect.NewError(connect.CodeAlreadyExists, errors.New(msg))
}
func PermissionDenied(msg string) error {
	return connect.NewError(connect.CodePermissionDenied, errors.New(msg))
}
func Unauthenticated(msg string) error {
	return connect.NewError(connect.CodeUnauthenticated, errors.New(msg))
}
func Unimplemented(msg string) error {
	return connect.NewError(connect.CodeUnimplemented, errors.New(msg))
}
func Unavailable(msg string) error { return connect.NewError(connect.CodeUnavailable, errors.New(msg)) }
func FailedPrecondition(msg string) error {
	return connect.NewError(connect.CodeFailedPrecondition, errors.New(msg))
}
func ResourceExhausted(msg string) error {
	return connect.NewError(connect.CodeResourceExhausted, errors.New(msg))
}

// internalError carries server-side context on an error whose client-visible
// message must stay generic.
//
// Error() is what the client sees, so it is the generic string. The detail and
// the cause are recoverable server-side through Detail and errors.Unwrap. The
// previous implementation simply discarded its argument, so call sites that
// wrote Internal("failed to write the commit") believed the text went
// somewhere and it went nowhere.
type internalError struct {
	detail string
	cause  error
}

func (e *internalError) Error() string { return "internal server error" }
func (e *internalError) Unwrap() error { return e.cause }

// Internal returns a CodeInternal error whose client-visible message is
// generic and whose detail is retained for logging. See Detail.
func Internal(detail string) error {
	return connect.NewError(connect.CodeInternal, &internalError{detail: detail})
}

// InternalCause is Internal with the underlying error attached, so
// errors.Is and errors.As still work on it after the boundary.
func InternalCause(detail string, cause error) error {
	return connect.NewError(connect.CodeInternal, &internalError{detail: detail, cause: cause})
}

// Detail returns the server-side detail carried by err, or "" when there is
// none. It is for logging: never put the result in a response.
func Detail(err error) string {
	var ie *internalError
	if !errors.As(err, &ie) {
		return ""
	}
	if ie.cause == nil {
		return ie.detail
	}
	return ie.detail + ": " + ie.cause.Error()
}

// ToConnectError ensures err is a *connect.Error. Errors already of that type
// are returned as-is; all others are wrapped as CodeInternal so internal
// details are never leaked to callers.
func ToConnectError(err error) error {
	if err == nil {
		return nil
	}

	var ce *connect.Error
	if errors.As(err, &ce) {
		return ce
	}

	return connect.NewError(connect.CodeInternal, errors.New("internal server error"))
}

// NewErrorInterceptor returns a Connect interceptor that translates any error a
// handler returns into a *connect.Error with a safe message.
//
// It is placed near the outside of the chain so that errors raised by the
// authorization, protovalidate and request-logging interceptors pass through it
// too.
func NewErrorInterceptor() connect.Interceptor {
	return errorInterceptor{}
}

type errorInterceptor struct{}

func (errorInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)
		return resp, ToConnectError(err)
	}
}

func (errorInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return ToConnectError(next(ctx, conn))
	}
}

func (errorInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

// Aborted reports a conflict the caller should resolve and retry, such as a
// compare-and-swap that lost.
func Aborted(msg string) error { return connect.NewError(connect.CodeAborted, errors.New(msg)) }
