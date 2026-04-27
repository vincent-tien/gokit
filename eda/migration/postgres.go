// Package migration exports DDL constants for the eda outbox/inbox tables.
//
// Projects copy/paste these constants into their own migration tooling
// (goose, atlas, golang-migrate, etc.). The migration package itself does
// not execute SQL — projects own their migration runner.
//
// Three tables are required for the full eda Phase 1+2 feature set:
//
//  1. outbox            — Writer.Store inserts, Relay reads + UPDATE published_at
//  2. inbox             — Inbox.Process INSERT ON CONFLICT DO NOTHING for dedup
//  3. inbox_dlq_retries — DLQ retry counter persisted across redeliveries
package migration

// PostgresOutbox creates the outbox table on Postgres.
//
// Columns:
//   - id           UUID v7 string (Writer sets via event.Meta.ID)
//   - topic        event topic for routing
//   - key          partition key (entity ID) — empty string allowed
//   - payload      whole event.Envelope JSON-marshaled
//   - headers      EventMeta.Headers JSON-marshaled
//   - created_at   set by Writer to event.Meta.OccurredAt
//   - published_at set by Relay when row is published; partial index on NULL for fast scan
const PostgresOutbox = `
CREATE TABLE IF NOT EXISTS outbox (
    id           VARCHAR(36) PRIMARY KEY,
    topic        VARCHAR(100) NOT NULL,
    key          VARCHAR(100) NOT NULL DEFAULT '',
    payload      JSONB NOT NULL,
    headers      JSONB DEFAULT '{}',
    created_at   TIMESTAMP NOT NULL DEFAULT NOW(),
    published_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox(created_at) WHERE published_at IS NULL;
`

// PostgresInbox creates the inbox dedup table on Postgres.
//
// Inbox.Process inserts ON CONFLICT (id) DO NOTHING — duplicates are silently
// skipped. retry_count + last_error are populated only by DLQ.Process when
// routing to DLQ; pure Inbox.Process leaves them at defaults.
const PostgresInbox = `
CREATE TABLE IF NOT EXISTS inbox (
    id           VARCHAR(36) PRIMARY KEY,
    processed_at TIMESTAMP NOT NULL DEFAULT NOW(),
    retry_count  INT NOT NULL DEFAULT 0,
    last_error   TEXT
);
`

// PostgresInboxDLQRetries persists the per-message retry counter outside
// the inbox tx. DLQ.Process upserts this row on every failed delivery so
// retry_count survives the inbox row rollback.
//
// Required by gokit/eda/outbox/dlq.go (Phase 5 design).
const PostgresInboxDLQRetries = `
CREATE TABLE IF NOT EXISTS inbox_dlq_retries (
    id          VARCHAR(36) PRIMARY KEY,
    retry_count INT NOT NULL,
    last_error  TEXT,
    updated_at  TIMESTAMP NOT NULL
);
`

// TODO E16: PostgresDistLock — added when distlock package lands (Phase 3 deferred).
