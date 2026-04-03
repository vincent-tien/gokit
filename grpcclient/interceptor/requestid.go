// Package interceptor provides gRPC client-side interceptors.
package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type requestIDKey struct{}

// WithRequestID stores a request ID in context for propagation.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext extracts the request ID from context.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

const requestIDHeader = "x-request-id"

// RequestID returns a unary interceptor that propagates X-Request-ID
// from context to outgoing gRPC metadata.
func RequestID() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if id := RequestIDFromContext(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, requestIDHeader, id)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamRequestID returns a stream interceptor that propagates X-Request-ID.
func StreamRequestID() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if id := RequestIDFromContext(ctx); id != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, requestIDHeader, id)
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}
