// Package broker provides message broker abstractions for event-driven architecture.
// Implementations: NATS JetStream (broker/nats.go).
package broker

import "context"

// Message represents a message to publish or received from a broker.
type Message struct {
	ID      string
	Topic   string
	Key     string
	Payload []byte
	Headers map[string]string
}

// Publisher sends messages to a broker.
type Publisher interface {
	Publish(ctx context.Context, msgs ...Message) error
	Close() error
}

// Handler processes a received message. Return nil to acknowledge.
type Handler func(ctx context.Context, msg Message) error

// Subscriber receives messages from a broker.
type Subscriber interface {
	Subscribe(ctx context.Context, topic string, handler Handler) error
	Close() error
}
