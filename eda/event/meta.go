// Package event defines the event envelope, metadata, and dispatchers used by
// the gokit/eda event-driven architecture.
package event

import (
	"time"

	"github.com/google/uuid"
)

// EventMeta carries metadata for every published event.
//
// Fields:
//   - ID: UUID v7 (time-ordered)
//   - Name: dot-separated entity.action, e.g. "invoice.published"
//   - Version: schema version (1, 2, ...). Additive changes only; breaking change → new event name
//   - OccurredAt: domain time when event happened (not publish time)
//   - CorrelationID: groups events from same user request across services
//   - CausationID: ID of the command/event that caused this event
//   - Source: producer service/module ("billing-service")
//   - Headers: extension slot for custom metadata (e.g. "entity_id" used by OutboxDispatcher)
type EventMeta struct {
	ID            string
	Name          string
	Version       int
	OccurredAt    time.Time
	CorrelationID string
	CausationID   string
	Source        string
	Headers       map[string]string
}

// NewMeta creates EventMeta with ID (UUID v7), OccurredAt (now), and the given name+version.
// Caller sets correlation, causation, source, headers afterwards via With* helpers or direct field access.
func NewMeta(name string, version int) EventMeta {
	return EventMeta{
		ID:         uuid.Must(uuid.NewV7()).String(),
		Name:       name,
		Version:    version,
		OccurredAt: time.Now(),
	}
}

// WithCorrelation returns a copy of m with the given correlation + causation IDs.
func (m EventMeta) WithCorrelation(correlationID, causationID string) EventMeta {
	m.CorrelationID = correlationID
	m.CausationID = causationID
	return m
}

// WithSource returns a copy of m with the given source service/module name.
func (m EventMeta) WithSource(source string) EventMeta {
	m.Source = source
	return m
}
