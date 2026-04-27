// Package edatest provides test helpers for projects using gokit/eda.
//
// RecordingBroker wraps the in-memory broker (broker.New("memory")) and
// records every published Message. Tests assert on Published() to verify
// event-publication behavior without standing up real infrastructure.
package edatest

import (
	"context"
	"sync"

	"github.com/vincent-tien/gokit/eda/broker"
)

// RecordingBroker decorates an in-memory broker with a thread-safe published log.
// Use NewMemoryBroker() to construct one; it satisfies both broker.Publisher
// and broker.Subscriber.
type RecordingBroker struct {
	pub  broker.Publisher
	sub  broker.Subscriber
	mu   sync.Mutex
	logs []broker.Message
}

// NewMemoryBroker creates a RecordingBroker backed by the registry's "memory"
// backend. The same RecordingBroker satisfies both Publisher and Subscriber so
// tests can wire it to producer + consumer paths simultaneously.
func NewMemoryBroker() *RecordingBroker {
	pub, sub, err := broker.New(broker.Config{Backend: "memory"})
	if err != nil {
		// memory backend is registered via init() in eda/broker/memory.go;
		// failure here means the broker package failed to initialize — panic.
		panic("edatest: memory broker not registered: " + err.Error())
	}
	return &RecordingBroker{pub: pub, sub: sub}
}

// Publish forwards to the underlying memory broker and records the messages.
func (rb *RecordingBroker) Publish(ctx context.Context, msgs ...broker.Message) error {
	rb.mu.Lock()
	rb.logs = append(rb.logs, msgs...)
	rb.mu.Unlock()
	return rb.pub.Publish(ctx, msgs...)
}

// Subscribe forwards to the underlying memory broker.
func (rb *RecordingBroker) Subscribe(ctx context.Context, topic string, h broker.Handler, opts ...broker.SubscribeOption) error {
	return rb.sub.Subscribe(ctx, topic, h, opts...)
}

// Close closes both the publisher and subscriber halves.
func (rb *RecordingBroker) Close() error {
	_ = rb.pub.Close()
	return rb.sub.Close()
}

// Published returns a snapshot copy of all messages published through this broker.
func (rb *RecordingBroker) Published() []broker.Message {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	out := make([]broker.Message, len(rb.logs))
	copy(out, rb.logs)
	return out
}

// Reset clears the recorded log. Useful between test cases when reusing a broker.
func (rb *RecordingBroker) Reset() {
	rb.mu.Lock()
	rb.logs = nil
	rb.mu.Unlock()
}
