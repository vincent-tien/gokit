package interceptor

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// Logger is a minimal logging interface for interceptor output.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

// Logging returns a unary interceptor that logs method, duration, and status code.
func Logging(log Logger) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		start := time.Now()
		err := invoker(ctx, method, req, reply, cc, opts...)
		duration := time.Since(start)

		st, _ := status.FromError(err)
		if err != nil {
			log.Error("grpc call failed",
				"method", method,
				"code", st.Code().String(),
				"duration", duration.String(),
				"error", err.Error(),
			)
		} else {
			log.Info("grpc call",
				"method", method,
				"code", st.Code().String(),
				"duration", duration.String(),
			)
		}
		return err
	}
}
