package broker

import (
	"context"
	"sync"
)

// memoryBroker is an in-process broker for testing and local development.
type memoryBroker struct {
	mu       sync.RWMutex
	handlers map[string][]memoryHandler
}

// memoryHandler holds a registered handler and its consumer group.
type memoryHandler struct {
	group   string
	handler Handler
}

// newMemory constructs a memoryBroker and returns it as both Publisher and Subscriber.
func newMemory(_ Config) (Publisher, Subscriber, error) {
	b := &memoryBroker{handlers: make(map[string][]memoryHandler)}
	return b, b, nil
}

// Publish delivers each message to all registered handlers for its topic.
// Within a non-empty consumer group only the first registered handler receives the message.
func (b *memoryBroker) Publish(ctx context.Context, msgs ...Message) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, msg := range msgs {
		delivered := map[string]bool{}
		for _, h := range b.handlers[msg.Topic] {
			if h.group != "" && delivered[h.group] {
				continue
			}
			_ = h.handler(ctx, msg) // memory: ignore handler errors (test fidelity)
			if h.group != "" {
				delivered[h.group] = true
			}
		}
	}
	return nil
}

// Subscribe registers handler for topic. opts may set a consumer group via WithGroup.
func (b *memoryBroker) Subscribe(_ context.Context, topic string, handler Handler, opts ...SubscribeOption) error {
	cfg := applyOpts(opts)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], memoryHandler{group: cfg.Group, handler: handler})
	return nil
}

// Close is a no-op for memoryBroker.
func (b *memoryBroker) Close() error { return nil }

func init() {
	Register("memory", newMemory)
}
