package interceptor

import (
	"context"

	"google.golang.org/grpc"

	"github.com/vincent-tien/gokit/breaker"
)

// CircuitBreaker returns a unary interceptor that wraps calls with a circuit breaker.
// Open circuit rejects calls immediately without calling the downstream service.
func CircuitBreaker(b breaker.Breaker) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return b.Execute(ctx, func() error {
			return invoker(ctx, method, req, reply, cc, opts...)
		})
	}
}
