package discovery

import "context"

type staticResolver struct {
	addresses map[string][]string
}

// Static returns a Resolver with a fixed address map. For dev/test.
//
//	r := discovery.Static(map[string][]string{
//	    "user-service": {"localhost:9001", "localhost:9002"},
//	})
func Static(addresses map[string][]string) Resolver {
	cp := make(map[string][]string, len(addresses))
	for k, v := range addresses {
		cp[k] = append([]string(nil), v...)
	}
	return &staticResolver{addresses: cp}
}

func (r *staticResolver) Resolve(_ context.Context, serviceName string) ([]string, error) {
	addrs, ok := r.addresses[serviceName]
	if !ok || len(addrs) == 0 {
		return nil, ErrNotFound
	}
	return addrs, nil
}
