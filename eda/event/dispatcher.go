package event

import (
	"context"
	"fmt"
	"sync"
)

// Dispatcher publishes events. Implementations:
//   - InProcessDispatcher: synchronous, in-memory (monolith — read-model updates, audit, same-process reactions)
//   - OutboxDispatcher: writes to outbox table for reliable cross-service delivery (Phase 6 in eda)
type Dispatcher interface {
	Dispatch(ctx context.Context, events ...Envelope) error
}

// InProcessDispatcher routes events to handlers registered with On.
// Handlers run synchronously in the calling goroutine, in registration order.
// First handler error halts the chain and is returned to the caller.
type InProcessDispatcher struct {
	mu       sync.RWMutex
	handlers map[string][]func(ctx context.Context, env Envelope) error
}

// NewInProcessDispatcher returns an empty dispatcher.
func NewInProcessDispatcher() *InProcessDispatcher {
	return &InProcessDispatcher{
		handlers: make(map[string][]func(ctx context.Context, env Envelope) error),
	}
}

// On registers handler for events with the given Name. Multiple handlers per name allowed.
func (d *InProcessDispatcher) On(eventName string, handler func(ctx context.Context, env Envelope) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[eventName] = append(d.handlers[eventName], handler)
}

// Dispatch invokes all handlers for each event's Name in registration order.
// First handler error halts and is returned wrapped with the event name.
func (d *InProcessDispatcher) Dispatch(ctx context.Context, events ...Envelope) error {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, env := range events {
		for _, h := range d.handlers[env.Meta.Name] {
			if err := h(ctx, env); err != nil {
				return fmt.Errorf("dispatch %s: %w", env.Meta.Name, err)
			}
		}
	}
	return nil
}
