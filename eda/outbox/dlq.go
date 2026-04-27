package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/vincent-tien/gokit/eda/broker"
)

// DLQ wraps Inbox with retry tracking and dead-letter routing.
//
// Design deviation from spec E12: retry counts are persisted in a separate
// "inbox_dlq_retries" table rather than columns on the "inbox" table. This
// keeps Inbox.Process (Phase 4) unchanged and avoids a tx-rollback race where
// incrementing retry_count inside the inbox tx would be rolled back along with
// the handler error.
//
// Required schema (add to Phase 7 migration SQL):
//
//	CREATE TABLE inbox_dlq_retries (
//	    id         TEXT PRIMARY KEY,
//	    retry_count INTEGER NOT NULL,
//	    last_error  TEXT,
//	    updated_at  TIMESTAMP NOT NULL
//	);
//
// Retry semantics:
//   - Attempts 1..(maxRetries-1): handler error → increment retry_count in
//     inbox_dlq_retries, return original err (broker redelivers).
//   - Attempt maxRetries: handler error → publish to dlqPrefix+msg.Topic with
//     diagnostic headers, insert tombstone into inbox (marks processed so further
//     redeliveries are skipped), return nil.
//   - Handler success (any attempt): delete retry tracker, return nil.
//
// Security note: dlqMsg.Headers["error"] carries hErr.Error(). Handlers should
// wrap internal errors with redacted strings before returning to avoid leaking
// sensitive data into DLQ headers.
type DLQ struct {
	inbox      *Inbox
	publisher  broker.Publisher
	maxRetries int
	dlqPrefix  string
	logger     Logger
}

// NewDLQ creates a DLQ wrapper. Defaults: maxRetries=3, dlqPrefix="dlq.", logger=nopLogger.
func NewDLQ(inbox *Inbox, pub broker.Publisher, opts ...DLQOption) *DLQ {
	d := &DLQ{
		inbox:      inbox,
		publisher:  pub,
		maxRetries: 3,
		dlqPrefix:  "dlq.",
		logger:     nopLogger{},
	}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Process invokes handler with retry tracking. On handler success, the retry
// tracker is cleaned up. On handler error, retry_count is incremented; once
// retry_count reaches maxRetries, the message is published to the DLQ topic
// and marked processed in the inbox so redeliveries are idempotently skipped.
func (d *DLQ) Process(ctx context.Context, msg broker.Message, handler func(ctx context.Context, tx *sql.Tx) error) error {
	// Read persisted retry_count (0 if first attempt).
	var retryCount int
	err := d.inbox.db.QueryRowContext(ctx,
		`SELECT retry_count FROM inbox_dlq_retries WHERE id = ?`, msg.ID,
	).Scan(&retryCount)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("dlq: read retry count for %s: %w", msg.ID, err)
	}

	// Delegate to Inbox.Process (handles exactly-once dedup via inbox table).
	hErr := d.inbox.Process(ctx, msg.ID, handler)
	if hErr == nil {
		// Success — remove retry tracker (best-effort; non-fatal if row absent).
		_, _ = d.inbox.db.ExecContext(ctx, `DELETE FROM inbox_dlq_retries WHERE id = ?`, msg.ID)
		return nil
	}

	// Handler failed. inbox.Process rolled back the inbox tx (no inbox row exists for msg.ID).
	newCount := retryCount + 1

	if newCount >= d.maxRetries {
		// Route to DLQ.
		dlqMsg := broker.Message{
			ID:      msg.ID,
			Topic:   d.dlqPrefix + msg.Topic,
			Key:     msg.Key,
			Payload: msg.Payload,
			Headers: copyHeaders(msg.Headers),
		}
		dlqMsg.Headers["original_topic"] = msg.Topic
		dlqMsg.Headers["error"] = hErr.Error()
		dlqMsg.Headers["retry_count"] = strconv.Itoa(newCount)

		if pubErr := d.publisher.Publish(ctx, dlqMsg); pubErr != nil {
			return fmt.Errorf("dlq: publish to %s: %w", dlqMsg.Topic, pubErr)
		}

		// Insert tombstone so Inbox.Process returns nil on future redeliveries
		// ("already processed" path — n == 0 branch in inbox.go).
		_, _ = d.inbox.db.ExecContext(ctx,
			`INSERT INTO inbox (id, processed_at) VALUES (?, CURRENT_TIMESTAMP) ON CONFLICT (id) DO NOTHING`,
			msg.ID,
		)
		// Remove retry tracker.
		_, _ = d.inbox.db.ExecContext(ctx, `DELETE FROM inbox_dlq_retries WHERE id = ?`, msg.ID)

		d.logger.Info("dlq: routed to dead-letter topic",
			"msg_id", msg.ID,
			"topic", dlqMsg.Topic,
			"after_retries", newCount,
		)
		return nil // caller must NOT nack; message is considered handled
	}

	// Increment retry tracker (UPSERT — works for both first failure and subsequent).
	_, err = d.inbox.db.ExecContext(ctx,
		`INSERT INTO inbox_dlq_retries (id, retry_count, last_error, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT (id) DO UPDATE
		     SET retry_count = excluded.retry_count,
		         last_error  = excluded.last_error,
		         updated_at  = excluded.updated_at`,
		msg.ID, newCount, hErr.Error(),
	)
	if err != nil {
		return fmt.Errorf("dlq: increment retry count for %s: %w", msg.ID, err)
	}

	d.logger.Info("dlq: handler error — will retry",
		"msg_id", msg.ID,
		"retry_count", newCount,
		"max_retries", d.maxRetries,
	)
	return hErr // bubble up so broker nacks/redelivers
}

// copyHeaders returns a shallow copy of in (nil-safe, always returns non-nil map).
func copyHeaders(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+3) // +3 for diagnostic keys
	for k, v := range in {
		out[k] = v
	}
	return out
}
