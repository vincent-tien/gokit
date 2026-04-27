// Package outbox provides transactional outbox write, relay, and inbox delivery.
//
// # AsyncAPI event catalog (spec L116)
//
// registry.go implements a process-local name+version registry for events produced
// or consumed by this service. Producers call [Register] (or [MustRegister]) in
// their init() or application boot:
//
//	func init() {
//	    outbox.MustRegister(outbox.RegisteredEvent{
//	        Name:    "invoice.published",
//	        Version: 1,
//	        Source:  "billing-service",
//	    })
//	}
//
// At publish time, callers may opt-in to validation:
//
//	if err := outbox.ValidateMeta(envelope.Meta); err != nil {
//	    return err
//	}
//
// [All] returns a stable snapshot suitable for AsyncAPI YAML export.
//
// Design choice — struct map key: the internal registry uses a composite struct
// {Name, Version} as the map key instead of a "name@vN" string. This avoids a
// latent gotcha where an event name containing the literal "@v" substring could
// produce a collision with a different (name, version) pair. Struct keys are
// comparable in Go and carry zero allocation overhead over strings.
package outbox

import (
	"fmt"
	"sort"
	"sync"

	"github.com/vincent-tien/gokit/eda/event"
)

// RegisteredEvent is a catalog entry for a single event name+version pair.
type RegisteredEvent struct {
	// Name is the dot-separated event identifier, e.g. "invoice.published".
	Name string
	// Version is the schema version, starting at 1. Additive changes only;
	// breaking changes must use a new Name or increment Version.
	Version int
	// Source is an optional hint identifying the producer module or service.
	Source string
}

// registryKey is the internal composite key used by the registry map.
type registryKey struct {
	name    string
	version int
}

var (
	registryMu sync.RWMutex
	registry   = map[registryKey]RegisteredEvent{}
)

// Register adds e to the catalog. It returns an error when:
//   - e.Name is empty
//   - e.Version is less than 1
//   - an entry with the same (Name, Version) pair is already registered
func Register(e RegisteredEvent) error {
	if e.Name == "" {
		return fmt.Errorf("outbox: event name must not be empty")
	}
	if e.Version < 1 {
		return fmt.Errorf("outbox: event %q version must be ≥1, got %d", e.Name, e.Version)
	}

	k := registryKey{name: e.Name, version: e.Version}

	registryMu.Lock()
	defer registryMu.Unlock()

	if _, exists := registry[k]; exists {
		return fmt.Errorf("outbox: event %q version %d is already registered", e.Name, e.Version)
	}

	registry[k] = e
	return nil
}

// MustRegister is like [Register] but panics on any error.
// Intended for use inside init() where silent failures are worse than a startup crash.
func MustRegister(e RegisteredEvent) {
	if err := Register(e); err != nil {
		panic(err)
	}
}

// Lookup returns the catalog entry for the given (name, version) pair.
// The second return value is false when the pair is not registered.
func Lookup(name string, version int) (RegisteredEvent, bool) {
	k := registryKey{name: name, version: version}

	registryMu.RLock()
	defer registryMu.RUnlock()

	e, ok := registry[k]
	return e, ok
}

// All returns a stable snapshot of every registered event, sorted by Name then
// Version. The slice is a copy — callers may freely modify it.
// Typical use: generate an AsyncAPI catalog from the running process.
func All() []RegisteredEvent {
	registryMu.RLock()
	out := make([]RegisteredEvent, 0, len(registry))
	for _, e := range registry {
		out = append(out, e)
	}
	registryMu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Version < out[j].Version
	})
	return out
}

// ValidateMeta checks that the (Name, Version) pair in meta is registered in the
// catalog. It returns nil when the pair is found and a descriptive error otherwise.
//
// This is an opt-in check — [Writer.Store] does NOT call ValidateMeta automatically.
// Wire it explicitly where you need the contract enforced.
func ValidateMeta(meta event.EventMeta) error {
	if _, ok := Lookup(meta.Name, meta.Version); !ok {
		return fmt.Errorf("outbox: event %q version %d is not registered in the catalog; call outbox.Register before publishing", meta.Name, meta.Version)
	}
	return nil
}

// Reset clears the registry.
//
// TEST ONLY — do not call in production code. Use t.Cleanup(outbox.Reset) to
// prevent test pollution between subtests.
func Reset() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[registryKey]RegisteredEvent{}
}
