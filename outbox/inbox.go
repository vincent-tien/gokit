package outbox

import (
	"context"
	"database/sql"
	"fmt"
)

// Inbox deduplicates incoming messages using a database table.
//
// Required table schema:
//
//	CREATE TABLE inbox (
//	    id           TEXT PRIMARY KEY,
//	    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
//	);
type Inbox struct {
	db *sql.DB
}

// NewInbox creates an Inbox deduplicator.
func NewInbox(db *sql.DB) *Inbox {
	return &Inbox{db: db}
}

// Process runs the handler only if msgID has not been processed before.
// Uses INSERT ... ON CONFLICT to atomically check-and-mark.
// Returns nil without calling handler if the message was already processed.
func (i *Inbox) Process(ctx context.Context, msgID string, handler func() error) error {
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("inbox: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Try to insert. If conflict → already processed.
	result, err := tx.ExecContext(ctx,
		"INSERT INTO inbox (id) VALUES (?) ON CONFLICT (id) DO NOTHING", msgID)
	if err != nil {
		return fmt.Errorf("inbox: insert %q: %w", msgID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inbox: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return nil // already processed
	}

	if err := handler(); err != nil {
		return fmt.Errorf("inbox: handler for %q: %w", msgID, err)
	}

	return tx.Commit()
}
