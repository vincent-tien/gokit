package discovery

import "fmt"

// Config holds the configuration for constructing a Resolver via New.
//
// Backend selects the resolver implementation (e.g. "static", "consul").
// Backend is required — discovery has no sensible default; callers must be explicit.
//
// Addr is the service address used by network-backed backends (e.g. Consul HTTP addr).
//
// Addresses is a static address map used by the "static" backend only.
//
// Options is a generic escape hatch for backend-specific config. For Consul advanced
// options (e.g. WithOnlyHealthy) use Consul(addr, opts...) directly — the factory
// path does not pass Options through to ConsulOption.
type Config struct {
	Backend   string
	Addr      string
	Addresses map[string][]string
	Options   map[string]any
}

// resolverFactory constructs a Resolver from a Config.
type resolverFactory func(Config) (Resolver, error)

var registry = map[string]resolverFactory{}

// Register adds a backend factory under the given name.
// Call from init() in each backend file.
func Register(name string, f resolverFactory) {
	registry[name] = f
}

// New constructs a Resolver for the backend named by cfg.Backend.
// cfg.Backend is required; an empty value returns an error listing registered backends.
// Returns an error if the backend is not registered.
//
// For Consul advanced options (e.g. WithOnlyHealthy), bypass New and call
// Consul(addr, opts...) directly — the factory path accepts only cfg.Addr.
func New(cfg Config) (Resolver, error) {
	if cfg.Backend == "" {
		return nil, fmt.Errorf("discovery: Backend is required (registered: %v)", registeredNames())
	}
	f, ok := registry[cfg.Backend]
	if !ok {
		return nil, fmt.Errorf("discovery: unknown backend %q (registered: %v)", cfg.Backend, registeredNames())
	}
	return f(cfg)
}

// registeredNames returns the sorted list of registered backend names.
func registeredNames() []string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return names
}
