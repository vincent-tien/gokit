package edatest

import (
	"database/sql"
	"testing"
)

// AssertEventPublished fails t if no message in rb's published log has
// Headers["event_name"] == eventName. Uses Headers populated by
// event.Envelope.ToBrokerMessage.
func AssertEventPublished(t *testing.T, rb *RecordingBroker, eventName string) {
	t.Helper()
	for _, msg := range rb.Published() {
		if msg.Headers["event_name"] == eventName {
			return
		}
	}
	t.Errorf("edatest: event %q not published; got %d messages in log", eventName, len(rb.Published()))
}

// AssertInboxProcessed fails t if no inbox row exists for msgID.
// Schema: assumes inbox table per migration.PostgresInbox / MySQLInbox.
func AssertInboxProcessed(t *testing.T, db *sql.DB, msgID string) {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM inbox WHERE id = ?", msgID).Scan(&count); err != nil {
		t.Fatalf("edatest: inbox query for %s: %v", msgID, err)
	}
	if count != 1 {
		t.Errorf("edatest: expected 1 inbox row for %s, got %d", msgID, count)
	}
}
