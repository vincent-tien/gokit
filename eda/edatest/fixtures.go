package edatest

import (
	"encoding/json"

	"github.com/google/uuid"

	"github.com/vincent-tien/gokit/eda/broker"
	"github.com/vincent-tien/gokit/eda/event"
)

// NewTestMessage returns a broker.Message with sensible defaults: random UUID v7
// for ID, the given topic, JSON-marshaled payload, and an empty Headers map.
func NewTestMessage(topic string, payload any) broker.Message {
	data, _ := json.Marshal(payload)
	return broker.Message{
		ID:      uuid.Must(uuid.NewV7()).String(),
		Topic:   topic,
		Payload: data,
		Headers: map[string]string{},
	}
}

// NewTestEnvelope returns an event.Envelope with NewMeta(name, 1) and the given
// payload. Convenient for tests that want a quick envelope without setting up
// Meta manually.
func NewTestEnvelope(name string, payload any) event.Envelope {
	env, _ := event.NewEnvelope(event.NewMeta(name, 1), payload)
	return env
}
