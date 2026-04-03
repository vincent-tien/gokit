// Package cache provides a cache abstraction with in-memory and Redis implementations.
//
// Use Memory for dev/test. Use Redis for production.
// GetOrLoad provides cache-aside with singleflight to prevent thundering herd.
package cache

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sync/singleflight"
)

// Cache is the core abstraction. Implementations: Memory (dev), Redis (prod).
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// ErrNotFound signals a cache miss. Consumers check: errors.Is(err, cache.ErrNotFound)
var ErrNotFound = errors.New("cache: key not found")

var group singleflight.Group

// GetOrLoad checks cache first. On miss, calls loader (deduplicated via singleflight),
// stores the result, and returns it. Prevents thundering herd on cold keys.
func GetOrLoad(ctx context.Context, c Cache, key string, ttl time.Duration, loader func(ctx context.Context) ([]byte, error)) ([]byte, error) {
	val, err := c.Get(ctx, key)
	if err == nil {
		return val, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	// Cache miss — load with singleflight dedup.
	result, err, _ := group.Do(key, func() (any, error) {
		data, err := loader(ctx)
		if err != nil {
			return nil, err
		}
		if setErr := c.Set(ctx, key, data, ttl); setErr != nil {
			return data, nil // return data even if cache set fails
		}
		return data, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]byte), nil
}
