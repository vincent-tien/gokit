// Package outbox implements the transactional outbox pattern for reliable event publishing.
//
// Writer stores events in the outbox table within the same database transaction
// as business data. Relay polls and publishes to a broker. Inbox deduplicates.
package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Event is a pending outbox event.
type Event struct {
	ID      string // auto-generated if empty
	Topic   string
	Key     string
	Payload []byte
}

// Writer stores events in the outbox table within the same transaction as business data.
type Writer struct {
	db *sql.DB
}

// NewWriter creates an outbox Writer.
func NewWriter(db *sql.DB) *Writer {
	return &Writer{db: db}
}

// Store inserts events into the outbox table within the given transaction.
// Events are picked up by a Relay worker for publishing.
//
// Required table schema:
//
//	CREATE TABLE outbox (
//	    id         TEXT PRIMARY KEY,
//	    topic      TEXT NOT NULL,
//	    key        TEXT NOT NULL DEFAULT '',
//	    payload    BYTEA NOT NULL,
//	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
//	);
func (w *Writer) Store(ctx context.Context, tx *sql.Tx, events ...Event) error {
	for i := range events {
		if events[i].ID == "" {
			events[i].ID = uuid.Must(uuid.NewV7()).String()
		}
		_, err := tx.ExecContext(ctx,
			"INSERT INTO outbox (id, topic, key, payload, created_at) VALUES (?, ?, ?, ?, ?)",
			events[i].ID, events[i].Topic, events[i].Key, events[i].Payload, time.Now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("outbox: store event %q: %w", events[i].ID, err)
		}
	}
	return nil
}
