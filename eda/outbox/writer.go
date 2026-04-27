package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vincent-tien/gokit/eda/event"
)

// Writer stores events in the outbox table within the same transaction
// as business data. Stateless — no DB pool field; tx supplies the connection.
//
// Required schema (Phase 7 publishes this in eda/migration/):
//
//	CREATE TABLE outbox (
//	  id TEXT PRIMARY KEY,
//	  topic TEXT NOT NULL,
//	  key TEXT NOT NULL DEFAULT '',
//	  payload BLOB/JSONB NOT NULL,
//	  headers BLOB/JSONB,
//	  created_at TIMESTAMP NOT NULL,
//	  published_at TIMESTAMP
//	);
type Writer struct{}

// NewWriter returns a stateless outbox Writer.
func NewWriter() *Writer { return &Writer{} }

// Store inserts envelopes into the outbox table within the given transaction.
// topic is the event topic for routing. key is the partition key (entity ID).
// Each envelope is JSON-marshaled into payload column. envelope.Meta.Headers
// are marshaled separately into the headers column. envelope.Meta.ID is the
// outbox row PK. envelope.Meta.OccurredAt is created_at (zero → time.Now()).
func (w *Writer) Store(ctx context.Context, tx *sql.Tx, topic, key string, events ...event.Envelope) error {
	for i := range events {
		data, err := json.Marshal(events[i])
		if err != nil {
			return fmt.Errorf("outbox: marshal envelope %s: %w", events[i].Meta.Name, err)
		}
		headers, err := json.Marshal(events[i].Meta.Headers)
		if err != nil {
			return fmt.Errorf("outbox: marshal headers %s: %w", events[i].Meta.Name, err)
		}
		occurredAt := events[i].Meta.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO outbox (id, topic, key, payload, headers, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			events[i].Meta.ID, topic, key, data, headers, occurredAt,
		)
		if err != nil {
			return fmt.Errorf("outbox: insert %s: %w", events[i].Meta.ID, err)
		}
	}
	return nil
}
