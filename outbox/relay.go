package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vincent-tien/gokit/broker"
)

// Relay polls the outbox table and publishes events to a broker.
type Relay struct {
	db        *sql.DB
	publisher broker.Publisher
	interval  time.Duration
	batchSize int
}

// RelayOption configures a Relay.
type RelayOption func(*Relay)

// WithInterval sets the polling interval. Default: 1 second.
func WithInterval(d time.Duration) RelayOption { return func(r *Relay) { r.interval = d } }

// WithBatchSize sets how many events to process per poll. Default: 100.
func WithBatchSize(n int) RelayOption { return func(r *Relay) { r.batchSize = n } }

// NewRelay creates a Relay that polls outbox and publishes to the given Publisher.
func NewRelay(db *sql.DB, pub broker.Publisher, opts ...RelayOption) *Relay {
	r := &Relay{
		db:        db,
		publisher: pub,
		interval:  time.Second,
		batchSize: 100,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Start begins polling the outbox table. Blocks until ctx is cancelled.
// Run in a goroutine: go relay.Start(ctx)
func (r *Relay) Start(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := r.poll(ctx); err != nil {
				// Log and continue — don't crash the relay on transient errors.
				continue
			}
		}
	}
}

func (r *Relay) poll(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("outbox relay: begin tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		"SELECT id, topic, key, payload FROM outbox ORDER BY created_at LIMIT ? FOR UPDATE SKIP LOCKED",
		r.batchSize,
	)
	if err != nil {
		return fmt.Errorf("outbox relay: query: %w", err)
	}
	defer rows.Close()

	var events []broker.Message
	var ids []any
	for rows.Next() {
		var m broker.Message
		if err := rows.Scan(&m.ID, &m.Topic, &m.Key, &m.Payload); err != nil {
			return fmt.Errorf("outbox relay: scan: %w", err)
		}
		events = append(events, m)
		ids = append(ids, m.ID)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("outbox relay: rows: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	if err := r.publisher.Publish(ctx, events...); err != nil {
		return fmt.Errorf("outbox relay: publish: %w", err)
	}

	// Delete published events one by one (portable SQL, works with ? placeholders).
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, "DELETE FROM outbox WHERE id = ?", id); err != nil {
			return fmt.Errorf("outbox relay: delete: %w", err)
		}
	}

	return tx.Commit()
}
