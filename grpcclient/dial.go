// Package grpcclient provides a gRPC client connection factory with sensible defaults.
package grpcclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

type config struct {
	insecure           bool
	tlsCertFile        string
	timeout            time.Duration
	keepaliveInterval  time.Duration
	keepaliveTimeout   time.Duration
	unaryInterceptors  []grpc.UnaryClientInterceptor
	streamInterceptors []grpc.StreamClientInterceptor
}

// Option configures a gRPC client connection.
type Option func(*config)

// WithInsecure disables TLS. Use for dev/internal services only.
func WithInsecure() Option { return func(c *config) { c.insecure = true } }

// WithTLS enables TLS using the given certificate file.
func WithTLS(certFile string) Option { return func(c *config) { c.tlsCertFile = certFile } }

// WithTimeout sets the connection timeout. Default: 5 seconds.
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithKeepalive configures keepalive parameters.
func WithKeepalive(interval, timeout time.Duration) Option {
	return func(c *config) {
		c.keepaliveInterval = interval
		c.keepaliveTimeout = timeout
	}
}

// WithUnaryInterceptors appends unary client interceptors.
func WithUnaryInterceptors(interceptors ...grpc.UnaryClientInterceptor) Option {
	return func(c *config) { c.unaryInterceptors = append(c.unaryInterceptors, interceptors...) }
}

// WithStreamInterceptors appends stream client interceptors.
func WithStreamInterceptors(interceptors ...grpc.StreamClientInterceptor) Option {
	return func(c *config) { c.streamInterceptors = append(c.streamInterceptors, interceptors...) }
}

// Dial creates a gRPC client connection with sensible defaults.
// Includes keepalive, connection timeout, optional TLS, and interceptor chain.
func Dial(target string, opts ...Option) (*grpc.ClientConn, error) {
	cfg := &config{
		timeout:           5 * time.Second,
		keepaliveInterval: 30 * time.Second,
		keepaliveTimeout:  10 * time.Second,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	dialOpts := []grpc.DialOption{
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                cfg.keepaliveInterval,
			Timeout:             cfg.keepaliveTimeout,
			PermitWithoutStream: true,
		}),
	}

	if cfg.insecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else if cfg.tlsCertFile != "" {
		creds, err := credentials.NewClientTLSFromFile(cfg.tlsCertFile, "")
		if err != nil {
			return nil, fmt.Errorf("grpcclient: load TLS cert: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	}

	if len(cfg.unaryInterceptors) > 0 {
		dialOpts = append(dialOpts, grpc.WithChainUnaryInterceptor(cfg.unaryInterceptors...))
	}
	if len(cfg.streamInterceptors) > 0 {
		dialOpts = append(dialOpts, grpc.WithChainStreamInterceptor(cfg.streamInterceptors...))
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: dial %q: %w", target, err)
	}
	return conn, nil
}
