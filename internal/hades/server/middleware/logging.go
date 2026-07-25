package middleware

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"github.com/alipourhabibi/Hades/utils/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// NewRequestLoggingInterceptor logs procedure, request headers, request body,
// response headers, response trailers, and any error at DEBUG level.
// Add to the base interceptor slice during development to trace buf CLI calls.
func NewRequestLoggingInterceptor(logger *log.LoggerWrapper) connect.UnaryInterceptorFunc {
	marshaler := protojson.MarshalOptions{EmitUnpopulated: false, UseProtoNames: true}
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			proc := req.Spec().Procedure

			var bodyJSON string
			if msg, ok := req.Any().(proto.Message); ok {
				if b, err := marshaler.Marshal(msg); err == nil {
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
				logger.Debug("rpc error",
					"procedure", proc,
					"error", err,
				)
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

func formatHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) == 1 {
			out[k] = vs[0]
		} else if len(vs) > 1 {
			out[k] = vs[0] // log first value; Authorization etc. are single-value
		}
	}
	return out
}
