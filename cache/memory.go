package cache

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	data      []byte
	expiresAt time.Time
}

func (e memEntry) expired() bool {
	return !e.expiresAt.IsZero() && time.Now().After(e.expiresAt)
}

// Memory is an in-memory Cache for dev/test. Concurrency-safe via sync.RWMutex.
// Not suitable for production multi-instance deployments.
type Memory struct {
	mu      sync.RWMutex
	entries map[string]memEntry
}

// NewMemory creates an in-memory cache.
func NewMemory() *Memory {
	return &Memory{entries: make(map[string]memEntry)}
}

func (m *Memory) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	entry, ok := m.entries[key]
	m.mu.RUnlock()

	if !ok || entry.expired() {
		if ok && entry.expired() {
			m.Delete(context.Background(), key)
		}
		return nil, ErrNotFound
	}
	// Return a copy to prevent mutation of cached data.
	cp := make([]byte, len(entry.data))
	copy(cp, entry.data)
	return cp, nil
}

func (m *Memory) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	cp := make([]byte, len(value))
	copy(cp, value)

	entry := memEntry{data: cp}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}

	m.mu.Lock()
	m.entries[key] = entry
	m.mu.Unlock()
	return nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.entries, key)
	m.mu.Unlock()
	return nil
}

func (m *Memory) Exists(_ context.Context, key string) (bool, error) {
	m.mu.RLock()
	entry, ok := m.entries[key]
	m.mu.RUnlock()

	if !ok || entry.expired() {
		return false, nil
	}
	return true, nil
}
