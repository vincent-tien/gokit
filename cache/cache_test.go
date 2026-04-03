package cache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Memory tests ──

func TestMemory_SetGet(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "k1", []byte("hello"), time.Minute))

	val, err := c.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), val)
}

func TestMemory_Get_Miss(t *testing.T) {
	c := NewMemory()

	_, err := c.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemory_Get_Expired(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "k1", []byte("data"), time.Millisecond))
	time.Sleep(5 * time.Millisecond)

	_, err := c.Get(ctx, "k1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemory_Get_NoTTL(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "k1", []byte("forever"), 0))

	val, err := c.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("forever"), val)
}

func TestMemory_Get_ReturnsCopy(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "k1", []byte("original"), time.Minute))

	val, _ := c.Get(ctx, "k1")
	val[0] = 'X' // mutate returned slice

	val2, _ := c.Get(ctx, "k1")
	assert.Equal(t, []byte("original"), val2, "cached data must not be mutated")
}

func TestMemory_Delete(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "k1", []byte("data"), time.Minute))
	require.NoError(t, c.Delete(ctx, "k1"))

	_, err := c.Get(ctx, "k1")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMemory_Exists(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	exists, err := c.Exists(ctx, "missing")
	require.NoError(t, err)
	assert.False(t, exists)

	require.NoError(t, c.Set(ctx, "k1", []byte("data"), time.Minute))

	exists, err = c.Exists(ctx, "k1")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestMemory_Exists_Expired(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "k1", []byte("data"), time.Millisecond))
	time.Sleep(5 * time.Millisecond)

	exists, err := c.Exists(ctx, "k1")
	require.NoError(t, err)
	assert.False(t, exists)
}

// ── GetOrLoad tests ──

func TestGetOrLoad_CacheHit(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()
	require.NoError(t, c.Set(ctx, "k1", []byte("cached"), time.Minute))

	loaderCalled := false
	val, err := GetOrLoad(ctx, c, "k1", time.Minute, func(_ context.Context) ([]byte, error) {
		loaderCalled = true
		return []byte("loaded"), nil
	})

	require.NoError(t, err)
	assert.Equal(t, []byte("cached"), val)
	assert.False(t, loaderCalled, "loader should not be called on cache hit")
}

func TestGetOrLoad_CacheMiss_LoadsAndCaches(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()

	val, err := GetOrLoad(ctx, c, "k1", time.Minute, func(_ context.Context) ([]byte, error) {
		return []byte("loaded"), nil
	})

	require.NoError(t, err)
	assert.Equal(t, []byte("loaded"), val)

	// Verify it was cached.
	cached, err := c.Get(ctx, "k1")
	require.NoError(t, err)
	assert.Equal(t, []byte("loaded"), cached)
}

func TestGetOrLoad_LoaderError(t *testing.T) {
	c := NewMemory()
	loaderErr := errors.New("db down")

	_, err := GetOrLoad(context.Background(), c, "k1", time.Minute, func(_ context.Context) ([]byte, error) {
		return nil, loaderErr
	})

	assert.ErrorIs(t, err, loaderErr)
}

func TestGetOrLoad_Singleflight_DeduplicatesCalls(t *testing.T) {
	c := NewMemory()
	ctx := context.Background()
	var callCount atomic.Int32

	loader := func(_ context.Context) ([]byte, error) {
		callCount.Add(1)
		time.Sleep(50 * time.Millisecond)
		return []byte("result"), nil
	}

	done := make(chan struct{}, 10)
	for range 10 {
		go func() {
			_, _ = GetOrLoad(ctx, c, "sf-key", time.Minute, loader)
			done <- struct{}{}
		}()
	}
	for range 10 {
		<-done
	}

	assert.LessOrEqual(t, callCount.Load(), int32(2), "singleflight should deduplicate concurrent calls")
}

// ── Interface satisfaction ──

func TestMemory_ImplementsCache(t *testing.T) {
	var _ Cache = NewMemory()
	var _ Cache = &Redis{}
	assert.True(t, true)
}
