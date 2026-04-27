package discovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_StaticBackend(t *testing.T) {
	cfg := Config{
		Backend: "static",
		Addresses: map[string][]string{
			"user-service": {"localhost:9001", "localhost:9002"},
		},
	}
	r, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, r)

	addrs, err := r.Resolve(context.Background(), "user-service")
	require.NoError(t, err)
	assert.Equal(t, []string{"localhost:9001", "localhost:9002"}, addrs)
}

func TestNew_UnknownBackend(t *testing.T) {
	_, err := New(Config{Backend: "nonexistent-backend-xyz"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown backend")
	assert.Contains(t, err.Error(), "registered")
}

func TestNew_NoBackend_ErrorsExplicitly(t *testing.T) {
	_, err := New(Config{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Backend is required")
	assert.Contains(t, err.Error(), "registered")
}

func TestRegister_AddsBackend(t *testing.T) {
	const fakeName = "__test_fake_discovery_backend__"
	defer delete(registry, fakeName)

	Register(fakeName, func(cfg Config) (Resolver, error) {
		return Static(map[string][]string{"svc": {"fake:1234"}}), nil
	})

	r, err := New(Config{Backend: fakeName})
	require.NoError(t, err)

	addrs, err := r.Resolve(context.Background(), "svc")
	require.NoError(t, err)
	assert.Equal(t, []string{"fake:1234"}, addrs)
}

func TestNew_ConsulBackend_FailsCleanly(t *testing.T) {
	// api.NewClient succeeds even for unreachable addresses; actual resolve fails at call time.
	r, err := New(Config{Backend: "consul", Addr: "127.0.0.1:0"})
	require.NoError(t, err)
	assert.NotNil(t, r)
}
