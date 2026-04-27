package migration

// MySQLOutbox creates the outbox table on MySQL/MariaDB.
//
// Differences from Postgres flavor:
//   - JSONB → JSON (MySQL 5.7+ has JSON, no JSONB)
//   - WHERE clause partial index NOT supported on MySQL → plain index on created_at
//   - `key` is a reserved word — backtick-quoted
const MySQLOutbox = "CREATE TABLE IF NOT EXISTS outbox (\n" +
	"    id           VARCHAR(36) PRIMARY KEY,\n" +
	"    topic        VARCHAR(100) NOT NULL,\n" +
	"    `key`        VARCHAR(100) NOT NULL DEFAULT '',\n" +
	"    payload      JSON NOT NULL,\n" +
	"    headers      JSON,\n" +
	"    created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,\n" +
	"    published_at TIMESTAMP NULL\n" +
	");\n" +
	"CREATE INDEX idx_outbox_unpublished ON outbox(created_at);\n"

// MySQLInbox creates the inbox dedup table on MySQL/MariaDB.
//
// Inbox.Process inserts ON CONFLICT equivalent — projects use
// INSERT IGNORE or INSERT ... ON DUPLICATE KEY UPDATE for MySQL dedup.
const MySQLInbox = `
CREATE TABLE IF NOT EXISTS inbox (
    id           VARCHAR(36) PRIMARY KEY,
    processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    retry_count  INT NOT NULL DEFAULT 0,
    last_error   TEXT
);
`

// MySQLInboxDLQRetries persists the per-message retry counter outside the
// inbox tx. DLQ.Process upserts this row on every failed delivery so
// retry_count survives the inbox row rollback.
//
// Required by gokit/eda/outbox/dlq.go (Phase 5 design).
const MySQLInboxDLQRetries = `
CREATE TABLE IF NOT EXISTS inbox_dlq_retries (
    id          VARCHAR(36) PRIMARY KEY,
    retry_count INT NOT NULL,
    last_error  TEXT,
    updated_at  TIMESTAMP NOT NULL
);
`

// TODO E16: MySQLDistLock — added when distlock package lands (Phase 3 deferred).
