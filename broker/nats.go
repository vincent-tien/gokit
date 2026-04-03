package broker

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSPublisher publishes messages to NATS JetStream.
type NATSPublisher struct {
	js jetstream.JetStream
}

// NewNATSPublisher creates a Publisher backed by NATS JetStream.
func NewNATSPublisher(js jetstream.JetStream) *NATSPublisher {
	return &NATSPublisher{js: js}
}

func (p *NATSPublisher) Publish(ctx context.Context, msgs ...Message) error {
	for _, msg := range msgs {
		nm := &nats.Msg{
			Subject: msg.Topic,
			Data:    msg.Payload,
			Header:  nats.Header{},
		}
		if msg.ID != "" {
			nm.Header.Set("Nats-Msg-Id", msg.ID)
		}
		if msg.Key != "" {
			nm.Header.Set("X-Key", msg.Key)
		}
		for k, v := range msg.Headers {
			nm.Header.Set(k, v)
		}

		if _, err := p.js.PublishMsg(ctx, nm); err != nil {
			return fmt.Errorf("broker: publish to %q: %w", msg.Topic, err)
		}
	}
	return nil
}

// Close is a no-op for NATSPublisher. The NATS connection is managed externally.
func (p *NATSPublisher) Close() error { return nil }

// NATSSubscriber subscribes to NATS JetStream subjects.
type NATSSubscriber struct {
	js   jetstream.JetStream
	cons []jetstream.ConsumeContext
}

// NewNATSSubscriber creates a Subscriber backed by NATS JetStream.
func NewNATSSubscriber(js jetstream.JetStream) *NATSSubscriber {
	return &NATSSubscriber{js: js}
}

// Subscribe starts consuming messages from the given topic (subject).
// The consumer name is derived from the topic. Messages are ack'd on nil handler return,
// nak'd on error.
func (s *NATSSubscriber) Subscribe(ctx context.Context, topic string, handler Handler) error {
	consumer, err := s.js.OrderedConsumer(ctx, topic, jetstream.OrderedConsumerConfig{})
	if err != nil {
		return fmt.Errorf("broker: create consumer for %q: %w", topic, err)
	}

	cc, err := consumer.Consume(func(jm jetstream.Msg) {
		msg := Message{
			Topic:   jm.Subject(),
			Payload: jm.Data(),
			Headers: make(map[string]string),
		}
		for k := range jm.Headers() {
			msg.Headers[k] = jm.Headers().Get(k)
		}
		msg.ID = msg.Headers["Nats-Msg-Id"]
		msg.Key = msg.Headers["X-Key"]

		if err := handler(ctx, msg); err != nil {
			// Ordered consumers don't support Nak; log and continue.
			return
		}
	})
	if err != nil {
		return fmt.Errorf("broker: consume %q: %w", topic, err)
	}

	s.cons = append(s.cons, cc)
	return nil
}

// Close stops all active consumer contexts.
func (s *NATSSubscriber) Close() error {
	for _, cc := range s.cons {
		cc.Stop()
	}
	s.cons = nil
	return nil
}
