// Package outbox implements the transactional outbox + inbox + DLQ patterns
// for reliable cross-service event delivery.
package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/vincent-tien/gokit/eda/broker"
)

// Relay polls the outbox table and publishes events to a broker. Adaptive
// polling: interval shrinks when batch is full, grows when idle.
//
// Production query uses `FOR UPDATE SKIP LOCKED` (Postgres/MySQL) so multiple
// relay instances can poll concurrently without blocking each other.
//
// Each Relay instance is single-goroutine: Start blocks until ctx is cancelled.
// Run multiple Relay instances (one per pod) for horizontal scale.
//
// TODO(R6): SKIP LOCKED semantics validated only via deferred Postgres
// integration test (see plans/260427-0816-gokit-eda-greenfield/phase-05).
// SQLite tests cover the marshal/publish/update path; multi-relay race tests
// require Postgres.
type Relay struct {
	db          *sql.DB
	publisher   broker.Publisher
	minInterval, maxInterval time.Duration
	batchSize   int
	logger      Logger

	// selectSQL is a test-only seam. nil → production query with FOR UPDATE
	// SKIP LOCKED. Tests inject a SQLite-compatible SELECT (no locking hint).
	// Never exported; set via package-internal helper in outbox_test.go.
	selectSQL *string
}

// defaultSelectSQL is the production query. FOR UPDATE SKIP LOCKED requires
// Postgres or MySQL — not supported by SQLite. Tests override via selectSQL seam.
const defaultSelectSQL = `SELECT id, topic, key, payload FROM outbox
WHERE published_at IS NULL
ORDER BY created_at ASC
LIMIT ?
FOR UPDATE SKIP LOCKED`

// NewRelay creates a Relay with default polling parameters:
// minInterval=10ms, maxInterval=1s, batchSize=100, logger=nopLogger.
func NewRelay(db *sql.DB, pub broker.Publisher, opts ...RelayOption) *Relay {
	r := &Relay{
		db:          db,
		publisher:   pub,
		minInterval: 10 * time.Millisecond,
		maxInterval: 1 * time.Second,
		batchSize:   100,
		logger:      nopLogger{},
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Start runs the relay loop until ctx is cancelled. Returns ctx.Err() on cancel.
//
// Adaptive interval state machine:
//   - batch full (n == batchSize) → interval = minInterval (more pending; poll fast)
//   - batch empty (n == 0)        → interval = min(interval*2, maxInterval) (idle; back off)
//   - batch partial               → interval = minInterval (still draining)
//   - poll error                  → interval = min(interval*2, maxInterval) (back off on errors)
func (r *Relay) Start(ctx context.Context) error {
	interval := r.maxInterval
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
			n, err := r.poll(ctx)
			if err != nil {
				r.logger.Error("outbox relay poll failed", "error", err)
				interval = backoff(interval, r.maxInterval)
				continue
			}
			switch {
			case n == r.batchSize:
				interval = r.minInterval
			case n == 0:
				interval = backoff(interval, r.maxInterval)
			default: // partial batch — still events pending
				interval = r.minInterval
			}
		}
	}
}

// backoff doubles d, capped at max.
func backoff(d, max time.Duration) time.Duration {
	doubled := d * 2
	if doubled > max {
		return max
	}
	return doubled
}

// poll selects up to batchSize unpublished rows, publishes them, then marks
// published_at = CURRENT_TIMESTAMP for each. Returns (rows-published, error).
//
// CURRENT_TIMESTAMP is portable across SQLite/Postgres/MySQL (unlike NOW()).
func (r *Relay) poll(ctx context.Context) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("relay: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	q := defaultSelectSQL
	if r.selectSQL != nil {
		q = *r.selectSQL
	}

	rows, err := tx.QueryContext(ctx, q, r.batchSize)
	if err != nil {
		return 0, fmt.Errorf("relay: query: %w", err)
	}

	msgs := make([]broker.Message, 0, r.batchSize)
	ids := make([]any, 0, r.batchSize)
	for rows.Next() {
		var m broker.Message
		if err := rows.Scan(&m.ID, &m.Topic, &m.Key, &m.Payload); err != nil {
			rows.Close()
			return 0, fmt.Errorf("relay: scan: %w", err)
		}
		msgs = append(msgs, m)
		ids = append(ids, m.ID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("relay: rows: %w", err)
	}
	rows.Close()

	if len(msgs) == 0 {
		return 0, nil
	}

	if err := r.publisher.Publish(ctx, msgs...); err != nil {
		return 0, fmt.Errorf("relay: publish: %w", err) // rollback unlocks rows for retry
	}

	// UPDATE published_at = CURRENT_TIMESTAMP WHERE id IN (?,?,?,...)
	// Placeholder count built dynamically to avoid SQL injection via column schema.
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	updateSQL := fmt.Sprintf(
		`UPDATE outbox SET published_at = CURRENT_TIMESTAMP WHERE id IN (%s)`,
		placeholders,
	)
	if _, err := tx.ExecContext(ctx, updateSQL, ids...); err != nil {
		return 0, fmt.Errorf("relay: update published_at: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("relay: commit: %w", err)
	}
	r.logger.Info("outbox relay published batch", "count", len(msgs))
	return len(msgs), nil
}
