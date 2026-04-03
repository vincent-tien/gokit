package testutil

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExecSQL executes raw SQL via *sql.DB. Works with both PostgreSQL and MySQL.
// Fails the test on error.
func ExecSQL(t testing.TB, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("testutil.ExecSQL: %v\nquery: %s", err, query)
	}
}

// ExecPgPool executes raw SQL via pgxpool (PostgreSQL only).
// Fails the test on error.
func ExecPgPool(t testing.TB, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("testutil.ExecPgPool: %v\nquery: %s", err, query)
	}
}
