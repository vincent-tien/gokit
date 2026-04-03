package discovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatic_Resolve_Found(t *testing.T) {
	r := Static(map[string][]string{
		"user-service": {"localhost:9001", "localhost:9002"},
		"order-service": {"localhost:9003"},
	})

	addrs, err := r.Resolve(context.Background(), "user-service")
	require.NoError(t, err)
	assert.Equal(t, []string{"localhost:9001", "localhost:9002"}, addrs)
}

func TestStatic_Resolve_NotFound(t *testing.T) {
	r := Static(map[string][]string{
		"user-service": {"localhost:9001"},
	})

	_, err := r.Resolve(context.Background(), "unknown-service")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestStatic_Resolve_EmptyAddresses(t *testing.T) {
	r := Static(map[string][]string{
		"empty-service": {},
	})

	_, err := r.Resolve(context.Background(), "empty-service")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestStatic_DefensiveCopy(t *testing.T) {
	original := map[string][]string{
		"svc": {"addr1"},
	}
	r := Static(original)

	// Mutate original — should not affect resolver.
	original["svc"][0] = "mutated"
	original["new"] = []string{"hack"}

	addrs, err := r.Resolve(context.Background(), "svc")
	require.NoError(t, err)
	assert.Equal(t, []string{"addr1"}, addrs, "resolver should have its own copy")

	_, err = r.Resolve(context.Background(), "new")
	assert.ErrorIs(t, err, ErrNotFound, "added key should not appear")
}

// Interface satisfaction.
func TestStatic_ImplementsResolver(t *testing.T) {
	var _ Resolver = Static(nil)
}
