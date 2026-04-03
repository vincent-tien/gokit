package interceptor

import (
	"context"
	"math"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryConfig configures the retry interceptor.
type RetryConfig struct {
	MaxRetries     int           // default: 3
	InitialBackoff time.Duration // default: 100ms
	RetryableCodes []codes.Code  // default: Unavailable, DeadlineExceeded
}

// Retry returns a unary interceptor with exponential backoff for transient errors.
func Retry(cfg RetryConfig) grpc.UnaryClientInterceptor {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 100 * time.Millisecond
	}
	if len(cfg.RetryableCodes) == 0 {
		cfg.RetryableCodes = []codes.Code{codes.Unavailable, codes.DeadlineExceeded}
	}

	retryable := make(map[codes.Code]bool, len(cfg.RetryableCodes))
	for _, c := range cfg.RetryableCodes {
		retryable[c] = true
	}

	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error
		for attempt := range cfg.MaxRetries {
			lastErr = invoker(ctx, method, req, reply, cc, opts...)
			if lastErr == nil {
				return nil
			}

			st, _ := status.FromError(lastErr)
			if !retryable[st.Code()] {
				return lastErr
			}

			if attempt < cfg.MaxRetries-1 {
				backoff := cfg.InitialBackoff * time.Duration(math.Pow(2, float64(attempt)))
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
			}
		}
		return lastErr
	}
}
