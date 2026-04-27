package event

import (
	"context"
	"database/sql"
	"fmt"
)

// TxFromContext extracts the active *sql.Tx from ctx. Implementations live
// outside eda — typically a project's tx-manager satisfies this directly or
// via a thin adapter. Returning ok=false signals "no tx active" — Dispatch
// will return an error instructing the caller to invoke within a tx scope.
//
// GoFrame's tx.Manager does NOT satisfy this interface directly (it returns a
// single *sql.Tx, not (*sql.Tx, bool)). Phase 9 provides a thin adapter that
// wraps tx.TxFromContext to add the bool return.
type TxFromContext interface {
	TxFromContext(ctx context.Context) (*sql.Tx, bool)
}

// Storer writes Envelopes into the outbox table within a tx. *outbox.Writer
// satisfies this interface structurally (Go's structural typing). Defined here
// in the event package to break the import cycle event↔outbox: outbox imports
// event for Envelope; event uses Storer (not outbox.Writer) so it does not
// import outbox in return.
type Storer interface {
	Store(ctx context.Context, tx *sql.Tx, topic, key string, events ...Envelope) error
}

// OutboxDispatcher writes events to the outbox table within the tx supplied
// by TxFromContext. It satisfies Dispatcher and is intended to be wired by the
// CQRS pipeline's CommandWithEvents step so that events commit atomically with
// business state.
//
// Construct via NewOutboxDispatcher. Caller MUST invoke Dispatch from within
// the tx scope established by their tx-manager (e.g., goframe's
// tx.Manager.WithTx). When no tx is active, Dispatch returns a descriptive
// error rather than silently writing outside the tx.
//
// For each event: topic = ev.Meta.Name; key = ev.Meta.Headers["entity_id"]
// (empty string if absent). The dispatcher applies WithSource(source) to every
// event before writing.
type OutboxDispatcher struct {
	writer Storer
	txCtx  TxFromContext
	source string
}

// NewOutboxDispatcher returns a dispatcher that writes via writer using the tx
// from txCtx. source is the producer service name set into each EventMeta.Source.
//
// writer is typically *outbox.Writer (it satisfies Storer structurally).
// txCtx is typically an adapter wrapping the project's tx manager.
func NewOutboxDispatcher(writer Storer, txCtx TxFromContext, source string) *OutboxDispatcher {
	return &OutboxDispatcher{writer: writer, txCtx: txCtx, source: source}
}

// Dispatch writes each event to the outbox via writer.Store. Returns an error
// if no tx is in context (TxFromContext.TxFromContext returns ok=false).
//
// Each event is written individually so different events can have different
// topics (Meta.Name) and partition keys (Headers["entity_id"]).
func (d *OutboxDispatcher) Dispatch(ctx context.Context, events ...Envelope) error {
	if len(events) == 0 {
		return nil
	}
	tx, ok := d.txCtx.TxFromContext(ctx)
	if !ok || tx == nil {
		return fmt.Errorf("event/outbox: no tx in context — Dispatch must run within a transaction scope")
	}
	for i := range events {
		events[i].Meta = events[i].Meta.WithSource(d.source)
		topic := events[i].Meta.Name
		key := events[i].Meta.Headers["entity_id"] // "" if absent — broker.Message.Key tolerates empty
		if err := d.writer.Store(ctx, tx, topic, key, events[i]); err != nil {
			return fmt.Errorf("event/outbox: store %s: %w", events[i].Meta.Name, err)
		}
	}
	return nil
}
