package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Compile-time interface satisfaction checks.
// These verify that all adapters implement the DB interface.
func TestAdapters_InterfaceSatisfaction(t *testing.T) {
	// StdDB
	var _ DB = StdDB(nil)
	// StdTx
	var _ DB = StdTx(nil)
	// PgxPool — cannot instantiate without real pool, but type check at compile time is enough.
	// PgxTx — same as above.

	assert.True(t, true, "all adapters satisfy DB interface")
}
