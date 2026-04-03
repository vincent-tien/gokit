package grpcclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestDial_Insecure(t *testing.T) {
	conn, err := Dial("localhost:0", WithInsecure())
	require.NoError(t, err)
	require.NotNil(t, conn)
	t.Cleanup(func() { conn.Close() })
}

func TestDial_WithTimeout(t *testing.T) {
	conn, err := Dial("localhost:0", WithInsecure(), WithTimeout(time.Second))
	require.NoError(t, err)
	require.NotNil(t, conn)
	t.Cleanup(func() { conn.Close() })
}

func TestDial_WithKeepalive(t *testing.T) {
	conn, err := Dial("localhost:0",
		WithInsecure(),
		WithKeepalive(15*time.Second, 5*time.Second),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)
	t.Cleanup(func() { conn.Close() })
}

func TestDial_WithInterceptors(t *testing.T) {
	noop := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	conn, err := Dial("localhost:0",
		WithInsecure(),
		WithUnaryInterceptors(noop),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)
	t.Cleanup(func() { conn.Close() })
}

func TestDial_TLS_InvalidCert(t *testing.T) {
	_, err := Dial("localhost:0", WithTLS("/nonexistent/cert.pem"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TLS cert")
}
