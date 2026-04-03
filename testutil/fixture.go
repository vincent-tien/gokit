package testutil

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FixedTime is a deterministic timestamp for test assertions.
// Always 2026-01-01T12:00:00Z — no timezone surprises.
var FixedTime = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// FixedTimeAt returns FixedTime + offset hours. Useful for ordering:
//
//	FixedTimeAt(1) = FixedTime + 1 hour
//	FixedTimeAt(2) = FixedTime + 2 hours
func FixedTimeAt(hoursOffset int) time.Time {
	return FixedTime.Add(time.Duration(hoursOffset) * time.Hour)
}

// UUID returns a deterministic, readable UUID for index n.
//
//	UUID(1)  → "00000000-0000-0000-0000-000000000001"
//	UUID(42) → "00000000-0000-0000-0000-000000000042"
//
// Same n always returns same value. Useful for stable test assertions.
func UUID(n int) string {
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", n)
}

// RandomUUID generates a real UUID v7 for tests needing uniqueness.
func RandomUUID() string {
	return uuid.Must(uuid.NewV7()).String()
}
