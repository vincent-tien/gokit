package broker

import (
	"context"
	"fmt"
	"sync"

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
		}
		if msg.ID != "" || msg.Key != "" || len(msg.Headers) > 0 {
			nm.Header = nats.Header{}
			if msg.ID != "" {
				nm.Header.Set("Nats-Msg-Id", msg.ID)
			}
			if msg.Key != "" {
				nm.Header.Set("X-Key", msg.Key)
			}
			for k, v := range msg.Headers {
				nm.Header.Set(k, v)
			}
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
	js  jetstream.JetStream
	mu  sync.Mutex
	cons []jetstream.ConsumeContext
}

// NewNATSSubscriber creates a Subscriber backed by NATS JetStream.
func NewNATSSubscriber(js jetstream.JetStream) *NATSSubscriber {
	return &NATSSubscriber{js: js}
}

// Subscribe starts consuming messages from the given topic (subject).
//
// LIMITATION: SubscribeOption arguments (group, concurrency, retries, ack-timeout)
// are currently IGNORED for the NATS backend. JetStream consumer-group /
// pull-subscribe option mapping is tracked under task E15. Callers that depend
// on these options should use the in-memory backend or wait for E15.
//
// Until E15 lands, all NATS subscriptions behave as ordered consumers with
// best-effort delivery. Multiple Subscribe calls on the same subject each
// create independent consumers.
func (s *NATSSubscriber) Subscribe(ctx context.Context, topic string, handler Handler, opts ...SubscribeOption) error {
	_ = opts // ignored until E15; see godoc above
	consumer, err := s.js.OrderedConsumer(ctx, topic, jetstream.OrderedConsumerConfig{})
	if err != nil {
		return fmt.Errorf("broker: create consumer for %q: %w", topic, err)
	}

	cc, err := consumer.Consume(func(jm jetstream.Msg) {
		msg := Message{
			Topic:   jm.Subject(),
			Payload: jm.Data(),
		}
		if h := jm.Headers(); h != nil && len(h) > 0 {
			msg.Headers = make(map[string]string, len(h))
			for k := range h {
				msg.Headers[k] = h.Get(k)
			}
			msg.ID = msg.Headers["Nats-Msg-Id"]
			msg.Key = msg.Headers["X-Key"]
		}

		_ = handler(ctx, msg)
	})
	if err != nil {
		return fmt.Errorf("broker: consume %q: %w", topic, err)
	}

	s.mu.Lock()
	s.cons = append(s.cons, cc)
	s.mu.Unlock()
	return nil
}

// Close stops all active consumer contexts.
func (s *NATSSubscriber) Close() error {
	s.mu.Lock()
	cons := s.cons
	s.cons = nil
	s.mu.Unlock()

	for _, cc := range cons {
		cc.Stop()
	}
	return nil
}
