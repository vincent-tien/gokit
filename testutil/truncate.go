package testutil

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Truncate clears tables for test isolation. Dialect-aware:
//
//	PostgreSQL: TRUNCATE TABLE t1, t2 RESTART IDENTITY CASCADE (single statement)
//	MySQL:      SET FOREIGN_KEY_CHECKS=0 → TRUNCATE per table → SET FOREIGN_KEY_CHECKS=1
//
// Accepts *sql.DB — works for both engines.
func Truncate(t testing.TB, db *sql.DB, tables ...string) {
	t.Helper()
	if len(tables) == 0 {
		return
	}

	dialect := DialectFromEnv()
	if dialect == MySQL {
		ExecSQL(t, db, "SET FOREIGN_KEY_CHECKS=0")
		for _, tbl := range tables {
			ExecSQL(t, db, fmt.Sprintf("TRUNCATE TABLE %s", tbl))
		}
		ExecSQL(t, db, "SET FOREIGN_KEY_CHECKS=1")
	} else {
		ExecSQL(t, db, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", ")))
	}
}

// TruncatePgPool is a convenience for pgxpool users (PostgreSQL only).
func TruncatePgPool(t testing.TB, pool *pgxpool.Pool, tables ...string) {
	t.Helper()
	if len(tables) == 0 {
		return
	}
	ExecPgPool(t, pool, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", ")))
}
