package broker

import "fmt"

// Config holds the configuration for constructing a broker via New.
type Config struct {
	Backend string
	Addr    string
	Options map[string]any
}

// factory is a constructor function registered for a backend name.
type factory func(Config) (Publisher, Subscriber, error)

var registry = map[string]factory{}

// Register adds a backend factory under the given name.
func Register(name string, f factory) {
	registry[name] = f
}

// New constructs a Publisher and Subscriber for the backend named by cfg.Backend.
// Returns an error if the backend is not registered.
func New(cfg Config) (Publisher, Subscriber, error) {
	f, ok := registry[cfg.Backend]
	if !ok {
		return nil, nil, fmt.Errorf("broker: unknown backend %q (registered: %v)", cfg.Backend, registeredNames())
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
