package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_DefaultBackend(t *testing.T) {
	c, err := New(Config{})
	require.NoError(t, err)
	assert.NotNil(t, c)
	_, ok := c.(*Memory)
	assert.True(t, ok, "empty Backend should return *Memory")
}

func TestNew_MemoryBackend(t *testing.T) {
	c, err := New(Config{Backend: "memory"})
	require.NoError(t, err)
	assert.NotNil(t, c)
	_, ok := c.(*Memory)
	assert.True(t, ok)
}

func TestNew_UnknownBackend(t *testing.T) {
	_, err := New(Config{Backend: "nope"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope")
	assert.Contains(t, err.Error(), "registered")
}

func TestRegister_AddsBackend(t *testing.T) {
	const fakeName = "test-fake-cache"
	defer delete(registry, fakeName)

	Register(fakeName, func(_ Config) (Cache, error) {
		return NewMemory(), nil
	})

	c, err := New(Config{Backend: fakeName})
	require.NoError(t, err)
	assert.NotNil(t, c)
}

func TestNew_RedisBackend(t *testing.T) {
	c, err := New(Config{Backend: "redis", Addr: "localhost:6379"})
	require.NoError(t, err)
	assert.NotNil(t, c)
	_, ok := c.(*Redis)
	assert.True(t, ok, "redis backend should return *Redis")
}
