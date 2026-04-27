package event

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/vincent-tien/gokit/eda/broker"
)

// Envelope wraps EventMeta and a JSON-encoded payload. It is the unit stored in
// the outbox table and published to brokers. Consumers parse Meta first
// (routing/version check), then call DecodePayload to get the typed body.
type Envelope struct {
	Meta    EventMeta       `json:"meta"`
	Payload json.RawMessage `json:"payload"`
}

// NewEnvelope marshals payload into json.RawMessage and returns the Envelope.
// Returns wrapped error if marshaling fails.
func NewEnvelope(meta EventMeta, payload any) (Envelope, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("event: marshal payload for %s: %w", meta.Name, err)
	}
	return Envelope{Meta: meta, Payload: data}, nil
}

// DecodePayload unmarshals e.Payload into dest. dest must be a non-nil pointer.
func (e Envelope) DecodePayload(dest any) error {
	return json.Unmarshal(e.Payload, dest)
}

// ToBrokerMessage marshals the entire Envelope to JSON and packages it as a
// broker.Message. Headers carry event_name/event_version/correlation_id/causation_id
// for transport-level filtering without parsing the body.
func (e Envelope) ToBrokerMessage(topic, key string) (broker.Message, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return broker.Message{}, fmt.Errorf("event: marshal envelope %s: %w", e.Meta.Name, err)
	}
	return broker.Message{
		ID:      e.Meta.ID,
		Topic:   topic,
		Key:     key,
		Payload: data,
		Headers: map[string]string{
			"event_name":     e.Meta.Name,
			"event_version":  strconv.Itoa(e.Meta.Version),
			"correlation_id": e.Meta.CorrelationID,
			"causation_id":   e.Meta.CausationID,
		},
	}, nil
}

// FromBrokerMessage parses an Envelope from broker.Message.Payload.
// Returns wrapped error if Payload is not a valid Envelope JSON.
func FromBrokerMessage(msg broker.Message) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(msg.Payload, &env); err != nil {
		return Envelope{}, fmt.Errorf("event: unmarshal envelope: %w", err)
	}
	return env, nil
}
