package middleware

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"connectrpc.com/connect"
	"github.com/alipourhabibi/Hades/utils/connerr"
	"github.com/alipourhabibi/Hades/utils/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// redactedPlaceholder is written in place of any value that must not reach a
// log file. It is deliberately visible rather than blank so that reading the
// log makes clear a value existed and was withheld.
const redactedPlaceholder = "[REDACTED]"

// loggableHeaders is the allowlist of headers whose values may be logged.
// Everything else is reported by name only. An allowlist is used rather than a
// denylist because a denylist silently fails to cover the next header that
// carries a credential: Authorization, Cookie, and any bespoke token header
// must never be written out, and neither must one nobody has thought of yet.
var loggableHeaders = map[string]struct{}{
	"accept":                    {},
	"accept-encoding":           {},
	"content-encoding":          {},
	"content-length":            {},
	"content-type":              {},
	"connect-accept-encoding":   {},
	"connect-content-encoding":  {},
	"connect-protocol-version":  {},
	"connect-timeout-ms":        {},
	"grpc-accept-encoding":      {},
	"grpc-encoding":             {},
	"grpc-status":               {},
	"grpc-message":              {},
	"grpc-timeout":              {},
	"traceparent":               {},
	"tracestate":                {},
	"user-agent":                {},
	"x-request-id":              {},
	"x-user-agent":              {},
	"buf-version":               {},
	"x-content-type-options":    {},
	"vary":                      {},
	"date":                      {},
	"connect-content-type":      {},
	"connection":                {},
	"transfer-encoding":         {},
	"strict-transport-security": {},
}

// sensitiveFieldTokens are substrings of protobuf field names whose values are
// never logged. Matching on the field name rather than on a per-message
// allowlist means a newly added credential field is redacted by default: the
// safe behaviour is what happens when nobody remembers to update this file.
var sensitiveFieldTokens = []string{
	"password",
	"passwd",
	"secret",
	"token",
	"hash",
	"credential",
	"private_key",
	"api_key",
	"apikey",
	"authorization",
	"signature",
	"otp",
	"_code",
	"code_",
}

// sensitiveFieldNames are field names redacted on an exact match. They are kept
// separate from the substring list because a substring rule for "code" or
// "state" would also blank unrelated fields such as a module's state enum.
var sensitiveFieldNames = map[string]struct{}{
	"code":  {}, // OAuth authorization code
	"state": {}, // OAuth state / CSRF nonce
	"nonce": {},
	"salt":  {},
}

// NewRequestLoggingInterceptor logs procedure, request headers, request body,
// response headers, response trailers, and any error at DEBUG level.
//
// Header values are allowlisted and message fields whose names look like
// credentials are redacted, so raising the log level to debug (the documented
// way to diagnose an RPC problem) does not start writing live passwords, TOTP
// codes and bearer tokens to disk.
func NewRequestLoggingInterceptor(logger *log.LoggerWrapper) connect.UnaryInterceptorFunc {
	marshaler := protojson.MarshalOptions{EmitUnpopulated: false, UseProtoNames: true}
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			proc := req.Spec().Procedure

			var bodyJSON string
			if msg, ok := req.Any().(proto.Message); ok {
				if b, err := marshaler.Marshal(redactMessage(msg)); err == nil {
					bodyJSON = string(b)
				}
			}

			logger.Debug("rpc request",
				"procedure", proc,
				"headers", formatHeaders(req.Header()),
				"body", bodyJSON,
			)

			resp, err := next(ctx, req)

			if err != nil {
				// A failed call is not debug output. Server faults are logged at
				// error so they appear at the default level; codes that mean the
				// caller got it wrong are logged at warn, because a stream of
				// those is a client problem rather than an incident.
				//
				// The error here has already been through the error interceptor,
				// so its message is the sanitised one. Whatever produced it is
				// expected to have logged the cause; see utils/connerr.FromDB.
				logAt := logger.Warn
				if isServerFault(connect.CodeOf(err)) {
					logAt = logger.Error
				}
				// connerr.Internal keeps the server-side detail attached to the
				// error rather than discarding it, so it is recovered here and
				// logged. It is never put in the response.
				args := []any{
					"procedure", proc,
					"code", connect.CodeOf(err).String(),
					"error", err,
				}
				if detail := connerr.Detail(err); detail != "" {
					args = append(args, "detail", detail)
				}
				logAt("rpc error", args...)
				return resp, err
			}

			logger.Debug("rpc response",
				"procedure", proc,
				"resp_headers", formatHeaders(resp.Header()),
				"resp_trailers", formatHeaders(resp.Trailer()),
			)

			return resp, err
		}
	}
}

// isServerFault reports whether a status code means the server failed rather
// than the caller having been refused or having sent something wrong.
func isServerFault(code connect.Code) bool {
	switch code {
	case connect.CodeInternal, connect.CodeUnknown, connect.CodeUnavailable,
		connect.CodeDataLoss, connect.CodeUnimplemented:
		return true
	default:
		return false
	}
}

// redactMessage returns a copy of msg with every field whose name looks like a
// credential cleared. The original is never modified, so the handler sees the
// message it was sent.
//
// A message with nothing to redact is returned as-is to avoid the clone.
func redactMessage(msg proto.Message) proto.Message {
	if msg == nil {
		return nil
	}
	m := msg.ProtoReflect()
	if !m.IsValid() {
		return msg
	}
	if !hasSensitiveField(m) {
		return msg
	}
	clone := proto.Clone(msg)
	redactInPlace(clone.ProtoReflect())
	return clone
}

// hasSensitiveField reports whether m or any nested message carries a populated
// field whose name matches sensitiveFieldTokens.
func hasSensitiveField(m protoreflect.Message) bool {
	found := false
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if isSensitiveFieldName(string(fd.Name())) {
			found = true
			return false
		}
		if fd.IsMap() || fd.Kind() != protoreflect.MessageKind && fd.Kind() != protoreflect.GroupKind {
			return true
		}
		if fd.IsList() {
			l := v.List()
			for i := 0; i < l.Len(); i++ {
				if hasSensitiveField(l.Get(i).Message()) {
					found = true
					return false
				}
			}
			return true
		}
		if hasSensitiveField(v.Message()) {
			found = true
			return false
		}
		return true
	})
	return found
}

// redactInPlace clears sensitive fields on m and recurses into nested messages.
func redactInPlace(m protoreflect.Message) {
	type clearing struct {
		fd     protoreflect.FieldDescriptor
		scalar bool
	}
	var toClear []clearing
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if isSensitiveFieldName(string(fd.Name())) {
			// A string field is replaced with a visible marker so the log shows
			// that a value was present; anything else is cleared outright.
			toClear = append(toClear, clearing{fd: fd, scalar: fd.Kind() == protoreflect.StringKind && !fd.IsList() && !fd.IsMap()})
			return true
		}
		if fd.IsMap() {
			return true
		}
		if fd.Kind() != protoreflect.MessageKind && fd.Kind() != protoreflect.GroupKind {
			return true
		}
		if fd.IsList() {
			l := v.List()
			for i := 0; i < l.Len(); i++ {
				redactInPlace(l.Get(i).Message())
			}
			return true
		}
		redactInPlace(v.Message())
		return true
	})
	for _, c := range toClear {
		if c.scalar {
			m.Set(c.fd, protoreflect.ValueOfString(redactedPlaceholder))
			continue
		}
		m.Clear(c.fd)
	}
}

// isSensitiveFieldName reports whether a protobuf field name contains any token
// that marks it as carrying a credential.
func isSensitiveFieldName(name string) bool {
	lower := strings.ToLower(name)
	if _, ok := sensitiveFieldNames[lower]; ok {
		return true
	}
	for _, tok := range sensitiveFieldTokens {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return false
}

// formatHeaders renders headers for the log. Values are included only for
// headers on the allowlist; everything else is recorded as a redaction marker
// so the header's presence is still visible.
func formatHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if _, ok := loggableHeaders[strings.ToLower(k)]; !ok {
			out[k] = redactedPlaceholder
			continue
		}
		switch {
		case len(vs) == 1:
			out[k] = vs[0]
		case len(vs) > 1:
			sorted := make([]string, len(vs))
			copy(sorted, vs)
			sort.Strings(sorted)
			out[k] = strings.Join(sorted, ",")
		}
	}
	return out
}
