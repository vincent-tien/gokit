package discovery

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul/api"
)

type consulResolver struct {
	client      *api.Client
	onlyHealthy bool
}

// ConsulOption configures a Consul resolver.
type ConsulOption func(*consulResolver)

// WithOnlyHealthy filters to only healthy service instances. Default: true.
func WithOnlyHealthy(v bool) ConsulOption {
	return func(r *consulResolver) { r.onlyHealthy = v }
}

// Consul returns a Resolver backed by Consul service discovery.
func Consul(addr string, opts ...ConsulOption) (Resolver, error) {
	cfg := api.DefaultConfig()
	cfg.Address = addr

	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("discovery: consul client: %w", err)
	}

	r := &consulResolver{
		client:      client,
		onlyHealthy: true,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

func (r *consulResolver) Resolve(_ context.Context, serviceName string) ([]string, error) {
	entries, _, err := r.client.Health().Service(serviceName, "", r.onlyHealthy, nil)
	if err != nil {
		return nil, fmt.Errorf("discovery: consul resolve %q: %w", serviceName, err)
	}
	if len(entries) == 0 {
		return nil, ErrNotFound
	}

	addrs := make([]string, 0, len(entries))
	for _, entry := range entries {
		addr := entry.Service.Address
		if addr == "" {
			addr = entry.Node.Address
		}
		addrs = append(addrs, fmt.Sprintf("%s:%d", addr, entry.Service.Port))
	}
	return addrs, nil
}
