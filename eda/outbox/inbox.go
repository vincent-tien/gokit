package outbox

import (
	"context"
	"database/sql"
	"fmt"
)

// Inbox deduplicates incoming messages using a database table.
// Process runs handler atomically with the inbox row insert: handler error
// rolls back BOTH the inbox row AND any writes the handler made via tx.
//
// Required schema (Phase 7 in eda/migration/):
//
//	CREATE TABLE inbox (
//	  id TEXT PRIMARY KEY,
//	  processed_at TIMESTAMP NOT NULL,
//	  retry_count INTEGER NOT NULL DEFAULT 0,
//	  last_error TEXT
//	);
type Inbox struct{ db *sql.DB }

// NewInbox creates an Inbox deduplicator backed by db.
func NewInbox(db *sql.DB) *Inbox { return &Inbox{db: db} }

// Process invokes handler atomically with the inbox row insert.
//   - First call with msgID: inserts inbox row, calls handler(ctx, tx), commits.
//   - Subsequent calls with same msgID: returns nil without calling handler (already processed).
//   - Handler error: tx rolls back, inbox row removed, broker can redeliver.
//
// Handler MUST do all its DB writes via tx. Calls outside tx are NOT atomic with
// the inbox row and may double-execute on redelivery.
func (i *Inbox) Process(ctx context.Context, msgID string, handler func(ctx context.Context, tx *sql.Tx) error) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("inbox: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	result, err := tx.ExecContext(ctx,
		`INSERT INTO inbox (id, processed_at) VALUES (?, CURRENT_TIMESTAMP) ON CONFLICT (id) DO NOTHING`,
		msgID,
	)
	if err != nil {
		return fmt.Errorf("inbox: insert %q: %w", msgID, err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inbox: rows affected: %w", err)
	}
	if n == 0 {
		return nil // already processed
	}

	if err := handler(ctx, tx); err != nil {
		return fmt.Errorf("inbox: handler for %q: %w", msgID, err)
	}
	return tx.Commit()
}
