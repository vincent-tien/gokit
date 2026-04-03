// Package discovery provides service discovery abstractions.
// Implementations: Static (dev/test), Consul (production).
package discovery

import (
	"context"
	"errors"
)

// ErrNotFound is returned when a service name has no known addresses.
var ErrNotFound = errors.New("discovery: service not found")

// Resolver finds service addresses by name.
type Resolver interface {
	Resolve(ctx context.Context, serviceName string) ([]string, error)
}
