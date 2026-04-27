package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/vincent-tien/gokit/eda/broker"
	"github.com/vincent-tien/gokit/eda/event"
)

// newSQLiteDB opens an in-memory SQLite DB and creates outbox + inbox +
// inbox_dlq_retries tables. All connections share the same in-memory DB via
// "file::memory:?cache=shared". t.Cleanup closes the DB.
func newSQLiteDB(t testing.TB) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping: %v", err)
	}
	db.SetMaxOpenConns(1) // SQLite memory DB: serialize to avoid "database is locked"
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS outbox (
			id           TEXT PRIMARY KEY,
			topic        TEXT NOT NULL,
			key          TEXT NOT NULL DEFAULT '',
			payload      BLOB NOT NULL,
			headers      BLOB,
			created_at   TIMESTAMP NOT NULL,
			published_at TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS inbox (
			id           TEXT PRIMARY KEY,
			processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			retry_count  INTEGER NOT NULL DEFAULT 0,
			last_error   TEXT
		);
		CREATE TABLE IF NOT EXISTS inbox_dlq_retries (
			id          TEXT PRIMARY KEY,
			retry_count INTEGER NOT NULL,
			last_error  TEXT,
			updated_at  TIMESTAMP NOT NULL
		);
	`)
	if err != nil {
		db.Close()
		t.Fatalf("create tables: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// mockPublisher records published messages for relay/DLQ testing.
type mockPublisher struct {
	msgs []broker.Message
	err  error
}

func (m *mockPublisher) Publish(_ context.Context, msgs ...broker.Message) error {
	if m.err != nil {
		return m.err
	}
	m.msgs = append(m.msgs, msgs...)
	return nil
}

func (m *mockPublisher) Close() error { return nil }

// sqliteSelectSQL is the SQLite-compatible substitute for the production
// "FOR UPDATE SKIP LOCKED" query. Selects the same columns so Scan is identical.
const sqliteSelectSQL = `SELECT id, topic, key, payload FROM outbox
WHERE published_at IS NULL
ORDER BY created_at ASC
LIMIT ?`

// setRelayTestSQL injects a SQLite-compatible SELECT into r and returns a
// restore function. Tests must defer the restore call.
func setRelayTestSQL(r *Relay, q string) func() {
	prev := r.selectSQL
	r.selectSQL = &q
	return func() { r.selectSQL = prev }
}

// ─── Constructor smoke tests ─────────────────────────────────────────────────

func TestNewWriter(t *testing.T) {
	w := NewWriter()
	assert.NotNil(t, w)
}

func TestNewInbox(t *testing.T) {
	inbox := NewInbox(nil)
	assert.NotNil(t, inbox)
}

// TestNewRelay_Defaults verifies the adaptive-polling defaults:
// minInterval=10ms, maxInterval=1s, batchSize=100, logger=nopLogger.
func TestNewRelay_Defaults(t *testing.T) {
	pub := &mockPublisher{}
	r := NewRelay(nil, pub)
	assert.Equal(t, 10*time.Millisecond, r.minInterval)
	assert.Equal(t, time.Second, r.maxInterval)
	assert.Equal(t, 100, r.batchSize)
	assert.IsType(t, nopLogger{}, r.logger)
}

// TestNewRelay_WithOptions verifies functional options override defaults.
func TestNewRelay_WithOptions(t *testing.T) {
	pub := &mockPublisher{}
	r := NewRelay(nil, pub,
		WithMinInterval(50*time.Millisecond),
		WithMaxInterval(5*time.Second),
		WithBatchSize(50),
	)
	assert.Equal(t, 50*time.Millisecond, r.minInterval)
	assert.Equal(t, 5*time.Second, r.maxInterval)
	assert.Equal(t, 50, r.batchSize)
}

// TestWithRelayLogger verifies WithRelayLogger installs the logger field.
func TestWithRelayLogger(t *testing.T) {
	pub := &mockPublisher{}
	var lg Logger = nopLogger{}
	r := NewRelay(nil, pub, WithRelayLogger(lg))
	assert.Equal(t, lg, r.logger)
}

func TestRelay_Start_RespectsContextCancellation(t *testing.T) {
	pub := &mockPublisher{}
	r := NewRelay(nil, pub, WithMaxInterval(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := r.Start(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ─── SQLite-backed Writer tests ──────────────────────────────────────────────

func TestWriter_Store_InsertsRow_Sqlite(t *testing.T) {
	db := newSQLiteDB(t)
	w := NewWriter()

	env, err := event.NewEnvelope(event.NewMeta("test.event", 1), map[string]any{"k": "v"})
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, w.Store(context.Background(), tx, "topic", "key-1", env))
	require.NoError(t, tx.Commit())

	var (
		id        string
		topic     string
		key       string
		payload   []byte
		headers   []byte
		createdAt time.Time
	)
	err = db.QueryRow(
		`SELECT id, topic, key, payload, headers, created_at FROM outbox WHERE id = ?`,
		env.Meta.ID,
	).Scan(&id, &topic, &key, &payload, &headers, &createdAt)
	require.NoError(t, err)

	assert.Equal(t, env.Meta.ID, id)
	assert.Equal(t, "topic", topic)
	assert.Equal(t, "key-1", key)
	assert.False(t, createdAt.IsZero())

	var got event.Envelope
	require.NoError(t, json.Unmarshal(payload, &got))
	assert.Equal(t, "test.event", got.Meta.Name)
	assert.Equal(t, env.Meta.ID, got.Meta.ID)
}

func TestInbox_Process_FirstCallRunsHandler(t *testing.T) {
	db := newSQLiteDB(t)
	inbox := NewInbox(db)

	called := false
	err := inbox.Process(context.Background(), "msg-1", func(ctx context.Context, tx *sql.Tx) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM inbox WHERE id = ?`, "msg-1").Scan(&n))
	assert.Equal(t, 1, n)
}

func TestInbox_Process_DuplicateSkipsHandler(t *testing.T) {
	db := newSQLiteDB(t)
	inbox := NewInbox(db)

	require.NoError(t, inbox.Process(context.Background(), "msg-2", func(ctx context.Context, tx *sql.Tx) error {
		return nil
	}))

	called := false
	err := inbox.Process(context.Background(), "msg-2", func(ctx context.Context, tx *sql.Tx) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.False(t, called, "handler must not run for duplicate msgID")
}

func TestInbox_Process_HandlerError_RollsBackInbox(t *testing.T) {
	db := newSQLiteDB(t)
	inbox := NewInbox(db)

	err := inbox.Process(context.Background(), "msg-3", func(ctx context.Context, tx *sql.Tx) error {
		return errors.New("boom")
	})
	require.Error(t, err)

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM inbox WHERE id = ?`, "msg-3").Scan(&n))
	assert.Equal(t, 0, n, "inbox row must be rolled back when handler errors")
}

func TestWriter_Store_MultipleEnvelopes(t *testing.T) {
	db := newSQLiteDB(t)
	w := NewWriter()

	env1, err := event.NewEnvelope(event.NewMeta("order.placed", 1), map[string]any{"id": 1})
	require.NoError(t, err)
	env2, err := event.NewEnvelope(event.NewMeta("order.paid", 1), map[string]any{"id": 2})
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, w.Store(context.Background(), tx, "orders", "ord-1", env1, env2))
	require.NoError(t, tx.Commit())

	var n int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM outbox`).Scan(&n))
	assert.Equal(t, 2, n)
}

func TestWriter_Store_ZeroOccurredAt(t *testing.T) {
	db := newSQLiteDB(t)
	w := NewWriter()

	meta := event.NewMeta("ping", 1)
	meta.OccurredAt = time.Time{} // force zero
	env, err := event.NewEnvelope(meta, struct{}{})
	require.NoError(t, err)

	tx, err := db.Begin()
	require.NoError(t, err)
	require.NoError(t, w.Store(context.Background(), tx, "pings", "p-1", env))
	require.NoError(t, tx.Commit())

	var raw string
	require.NoError(t, db.QueryRow(`SELECT created_at FROM outbox WHERE id = ?`, env.Meta.ID).Scan(&raw))
	assert.NotEmpty(t, raw)
}

func TestInbox_Process_HandlerCanWriteViaTx(t *testing.T) {
	db := newSQLiteDB(t)
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS side_effect (val TEXT PRIMARY KEY)`)
	require.NoError(t, err)

	inbox := NewInbox(db)
	err = inbox.Process(context.Background(), "msg-tx-1", func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO side_effect (val) VALUES (?)`, "written")
		return err
	})
	require.NoError(t, err)

	var val string
	require.NoError(t, db.QueryRow(`SELECT val FROM side_effect`).Scan(&val))
	assert.Equal(t, "written", val)
}

// ─── Relay poll tests (SQLite-backed) ────────────────────────────────────────

// TestRelay_poll_EmptyTable covers the zero-events early-return path.
func TestRelay_poll_EmptyTable(t *testing.T) {
	db := newSQLiteDB(t)
	pub := &mockPublisher{}
	r := NewRelay(db, pub)
	defer setRelayTestSQL(r, sqliteSelectSQL)()

	n, err := r.poll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Empty(t, pub.msgs, "no messages published for empty outbox")
}

// TestRelay_Poll_PublishesAndMarksRows_Sqlite inserts 3 rows, runs poll, and
// verifies all 3 are published and marked published_at IS NOT NULL (UPDATE, not DELETE).
func TestRelay_Poll_PublishesAndMarksRows_Sqlite(t *testing.T) {
	db := newSQLiteDB(t)
	pub := &mockPublisher{}
	r := NewRelay(db, pub, WithBatchSize(10))
	defer setRelayTestSQL(r, sqliteSelectSQL)()

	for i, id := range []string{"evt-1", "evt-2", "evt-3"} {
		_, err := db.Exec(
			`INSERT INTO outbox (id, topic, key, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
			id, "orders", fmt.Sprintf("ord-%d", i+1), []byte(`{"test":true}`),
			time.Now().UTC().Add(time.Duration(i)*time.Millisecond),
		)
		require.NoError(t, err)
	}

	n, err := r.poll(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	assert.Len(t, pub.msgs, 3)

	// All rows must now have published_at set (UPDATE — not DELETE).
	var unpublished int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE published_at IS NULL`).Scan(&unpublished))
	assert.Equal(t, 0, unpublished, "all rows must have published_at set after poll")

	var total int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM outbox`).Scan(&total))
	assert.Equal(t, 3, total, "rows must NOT be deleted — only published_at marked")
}

// TestRelay_Poll_RollbacksOnPublishError verifies rows remain unpublished when
// broker.Publish fails (tx rollback).
func TestRelay_Poll_RollbacksOnPublishError(t *testing.T) {
	db := newSQLiteDB(t)
	pub := &mockPublisher{err: errors.New("broker down")}
	r := NewRelay(db, pub)
	defer setRelayTestSQL(r, sqliteSelectSQL)()

	_, err := db.Exec(
		`INSERT INTO outbox (id, topic, key, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
		"evt-fail", "orders", "ord-fail", []byte(`{}`), time.Now().UTC(),
	)
	require.NoError(t, err)

	n, pollErr := r.poll(context.Background())
	require.Error(t, pollErr)
	assert.Equal(t, 0, n)
	assert.Contains(t, pollErr.Error(), "relay: publish")

	var unpublished int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM outbox WHERE published_at IS NULL`).Scan(&unpublished))
	assert.Equal(t, 1, unpublished, "row must remain unpublished — tx was rolled back")
}

// TestRelay_Start_PollError covers the Start error-log + backoff branch: poll
// fails (SQLite rejects FOR UPDATE SKIP LOCKED), Start keeps running until ctx cancel.
func TestRelay_Start_PollError(t *testing.T) {
	db := newSQLiteDB(t)
	pub := &mockPublisher{}
	// No selectSQL override → production query with FOR UPDATE SKIP LOCKED → fails on SQLite.
	r := NewRelay(db, pub, WithMinInterval(5*time.Millisecond), WithMaxInterval(10*time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := r.Start(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		"expected context error, got: %v", err)
}

// ─── Adaptive interval unit tests ────────────────────────────────────────────

// TestRelay_Adaptive_BackoffOnError verifies the backoff() helper doubles d,
// capped at max.
func TestRelay_Adaptive_BackoffOnError(t *testing.T) {
	cases := []struct {
		d, max, want time.Duration
	}{
		{10 * time.Millisecond, time.Second, 20 * time.Millisecond},
		{500 * time.Millisecond, time.Second, time.Second},
		{time.Second, time.Second, time.Second},
		{600 * time.Millisecond, time.Second, time.Second},
		{1 * time.Millisecond, 100 * time.Millisecond, 2 * time.Millisecond},
	}
	for _, tc := range cases {
		got := backoff(tc.d, tc.max)
		assert.Equal(t, tc.want, got, "backoff(%v, %v)", tc.d, tc.max)
	}
}

// TestRelay_Adaptive_StateTransitions table-drives the interval state machine
// logic from Start(), covering full/partial/empty/error outcomes.
func TestRelay_Adaptive_StateTransitions(t *testing.T) {
	const minI = 10 * time.Millisecond
	const maxI = 1 * time.Second

	cases := []struct {
		name         string
		pollN        int
		pollErr      bool
		prevInterval time.Duration
		wantInterval time.Duration
	}{
		{
			name: "full batch → minInterval",
			pollN: 100, prevInterval: maxI, wantInterval: minI,
		},
		{
			name: "partial batch → minInterval",
			pollN: 42, prevInterval: maxI, wantInterval: minI,
		},
		{
			name: "empty → double previous (under max)",
			pollN: 0, prevInterval: 100 * time.Millisecond, wantInterval: 200 * time.Millisecond,
		},
		{
			name: "empty → capped at max",
			pollN: 0, prevInterval: 800 * time.Millisecond, wantInterval: maxI,
		},
		{
			name: "error → double (under max)",
			pollErr: true, prevInterval: 50 * time.Millisecond, wantInterval: 100 * time.Millisecond,
		},
		{
			name: "error → capped at max",
			pollErr: true, prevInterval: maxI, wantInterval: maxI,
		},
	}

	r := &Relay{minInterval: minI, maxInterval: maxI, batchSize: 100}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interval := tc.prevInterval
			n := tc.pollN
			var err error
			if tc.pollErr {
				err = errors.New("poll error")
			}

			// Mirror the state machine from Start().
			if err != nil {
				interval = backoff(interval, r.maxInterval)
			} else {
				switch {
				case n == r.batchSize:
					interval = r.minInterval
				case n == 0:
					interval = backoff(interval, r.maxInterval)
				default:
					interval = r.minInterval
				}
			}
			assert.Equal(t, tc.wantInterval, interval, tc.name)
		})
	}
}

// ─── DLQ tests (SQLite-backed) ───────────────────────────────────────────────

// TestDLQ_RetryIncrementsCount verifies retry_count in inbox_dlq_retries
// increments on successive handler failures before maxRetries is reached.
func TestDLQ_RetryIncrementsCount(t *testing.T) {
	db := newSQLiteDB(t)
	inbox := NewInbox(db)
	pub := &mockPublisher{}
	dlq := NewDLQ(inbox, pub, WithMaxRetries(3))

	handlerErr := errors.New("transient failure")
	msg := broker.Message{ID: "dlq-msg-1", Topic: "orders", Payload: []byte(`{}`)}

	// First failure → retry_count = 1.
	err := dlq.Process(context.Background(), msg, func(ctx context.Context, tx *sql.Tx) error {
		return handlerErr
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "transient failure")

	var count1 int
	require.NoError(t, db.QueryRow(`SELECT retry_count FROM inbox_dlq_retries WHERE id = ?`, msg.ID).Scan(&count1))
	assert.Equal(t, 1, count1)

	// Second failure → retry_count = 2.
	err = dlq.Process(context.Background(), msg, func(ctx context.Context, tx *sql.Tx) error {
		return handlerErr
	})
	require.Error(t, err)

	var count2 int
	require.NoError(t, db.QueryRow(`SELECT retry_count FROM inbox_dlq_retries WHERE id = ?`, msg.ID).Scan(&count2))
	assert.Equal(t, 2, count2)
}

// TestWithDLQLogger verifies WithDLQLogger installs the logger field.
func TestWithDLQLogger(t *testing.T) {
	db := newSQLiteDB(t)
	inbox := NewInbox(db)
	pub := &mockPublisher{}
	var lg Logger = nopLogger{}
	dlq := NewDLQ(inbox, pub, WithDLQLogger(lg))
	assert.Equal(t, lg, dlq.logger)
}

// TestNopLogger_InfoError calls nopLogger methods to ensure they are reachable
// (drives coverage of the no-op body lines).
func TestNopLogger_InfoError(t *testing.T) {
	var l Logger = nopLogger{}
	// Neither call panics or returns anything — just prove the lines execute.
	l.Info("msg", "k", "v")
	l.Error("msg", "k", "v")
}

// TestDLQ_Process_SuccessDeletesRetryTracker verifies that when the handler
// succeeds, the retry tracker row (if any) is cleaned up.
func TestDLQ_Process_SuccessDeletesRetryTracker(t *testing.T) {
	db := newSQLiteDB(t)
	inbox := NewInbox(db)
	pub := &mockPublisher{}
	dlq := NewDLQ(inbox, pub)

	msg := broker.Message{ID: "dlq-success-1", Topic: "orders", Payload: []byte(`{}`)}

	// Pre-seed a tracker row (simulates a prior failed attempt).
	_, err := db.Exec(
		`INSERT INTO inbox_dlq_retries (id, retry_count, last_error, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		msg.ID, 1, "previous error",
	)
	require.NoError(t, err)

	// Handler succeeds → retry tracker must be deleted.
	err = dlq.Process(context.Background(), msg, func(ctx context.Context, tx *sql.Tx) error {
		return nil
	})
	require.NoError(t, err)

	var n int
	err = db.QueryRow(`SELECT COUNT(*) FROM inbox_dlq_retries WHERE id = ?`, msg.ID).Scan(&n)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "retry tracker must be deleted on success")
}

// TestRelay_Start_FullBatchTransition exercises the Start success path with a
// real SQLite poll: inserts batchSize rows so n == batchSize → minInterval,
// then cancels ctx. Verifies Start returns a context error (not a poll error).
func TestRelay_Start_FullBatchTransition(t *testing.T) {
	db := newSQLiteDB(t)
	pub := &mockPublisher{}
	r := NewRelay(db, pub,
		WithBatchSize(2),
		WithMinInterval(5*time.Millisecond),
		WithMaxInterval(20*time.Millisecond),
	)
	defer setRelayTestSQL(r, sqliteSelectSQL)()

	// Insert exactly batchSize rows so first poll returns n == batchSize.
	for i := 0; i < 2; i++ {
		_, err := db.Exec(
			`INSERT INTO outbox (id, topic, key, payload, created_at) VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("full-%d", i), "orders", fmt.Sprintf("k-%d", i),
			[]byte(`{}`), time.Now().UTC().Add(time.Duration(i)*time.Millisecond),
		)
		require.NoError(t, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	err := r.Start(ctx)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
}

// TestDLQ_RetryThenForward_Sqlite verifies that on the maxRetries-th failure
// the message is published to the DLQ topic and Process returns nil (no nack).
// Diagnostic headers and inbox tombstone must be present; retry tracker removed.
func TestDLQ_RetryThenForward_Sqlite(t *testing.T) {
	db := newSQLiteDB(t)
	inbox := NewInbox(db)
	pub := &mockPublisher{}
	dlq := NewDLQ(inbox, pub, WithMaxRetries(3), WithDLQPrefix("dead."))

	handlerErr := errors.New("unrecoverable")
	msg := broker.Message{
		ID:      "dlq-msg-2",
		Topic:   "payments",
		Payload: []byte(`{"amount":100}`),
		Headers: map[string]string{"X-Tenant": "acme"},
	}

	// Pre-seed 2 prior failures in the tracker (simulates 2 previous redeliveries).
	_, err := db.Exec(
		`INSERT INTO inbox_dlq_retries (id, retry_count, last_error, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`,
		msg.ID, 2, "prev error",
	)
	require.NoError(t, err)

	// Third attempt (newCount == maxRetries == 3) → must route to DLQ and return nil.
	err = dlq.Process(context.Background(), msg, func(ctx context.Context, tx *sql.Tx) error {
		return handlerErr
	})
	require.NoError(t, err, "DLQ route must return nil so broker does not nack")

	// Verify DLQ message published to correct topic.
	require.Len(t, pub.msgs, 1)
	dlqMsg := pub.msgs[0]
	assert.Equal(t, "dead.payments", dlqMsg.Topic)
	assert.Equal(t, msg.ID, dlqMsg.ID)
	assert.Equal(t, "payments", dlqMsg.Headers["original_topic"])
	// Inbox.Process wraps the handler error: "inbox: handler for %q: <msg>"
	assert.Contains(t, dlqMsg.Headers["error"], "unrecoverable")
	assert.Equal(t, "3", dlqMsg.Headers["retry_count"])
	assert.Equal(t, "acme", dlqMsg.Headers["X-Tenant"], "original headers must be preserved")

	// Inbox tombstone must exist so future redeliveries skip the handler.
	var inboxCount int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM inbox WHERE id = ?`, msg.ID).Scan(&inboxCount))
	assert.Equal(t, 1, inboxCount, "inbox tombstone must be inserted after DLQ route")

	// Retry tracker must be cleaned up.
	var retryCount int
	err = db.QueryRow(`SELECT retry_count FROM inbox_dlq_retries WHERE id = ?`, msg.ID).Scan(&retryCount)
	assert.Equal(t, sql.ErrNoRows, err, "retry tracker must be deleted after DLQ route")
}
