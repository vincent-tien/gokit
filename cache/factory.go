package cache

import (
	"fmt"
	"time"
)

// Config holds the configuration for constructing a Cache via New.
// Addr is used by the redis factory to build a *redis.Client.
// TTL is a caller-level default; individual Cache.Set calls specify per-call TTL.
type Config struct {
	Backend string
	Addr    string
	Options map[string]any
	TTL     time.Duration
}

// cacheFactory is a constructor function registered for a backend name.
type cacheFactory func(Config) (Cache, error)

var registry = map[string]cacheFactory{}

// Register adds a backend factory under the given name.
// Call from init() in each backend file.
func Register(name string, f cacheFactory) {
	registry[name] = f
}

// New constructs a Cache for the backend named by cfg.Backend.
// If cfg.Backend is empty, "memory" is used.
// Returns an error if the backend is not registered.
//
// Note: the redis factory builds a *redis.Client from cfg.Addr without calling
// Ping. Validate cfg.Addr before passing to New. To share connection pools,
// use NewRedis directly.
func New(cfg Config) (Cache, error) {
	backend := cfg.Backend
	if backend == "" {
		backend = "memory"
	}
	f, ok := registry[backend]
	if !ok {
		return nil, fmt.Errorf("cache: unknown backend %q (registered: %v)", backend, registeredNames())
	}
	return f(cfg)
}

// registeredNames returns the list of registered backend names.
func registeredNames() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}
