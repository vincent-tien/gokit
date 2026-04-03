package testutil

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql" // MySQL/MariaDB driver
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB creates a *sql.DB connection to the test database.
// Works with BOTH PostgreSQL and MySQL via database/sql.
// Reads DATABASE_URL from env. Falls back to dialect-specific localhost DSN.
// Connection is closed via t.Cleanup.
func DB(t testing.TB, defaultDSN ...string) *sql.DB {
	t.Helper()
	dialect := DialectFromEnv()
	dsn := resolveDSN(dialect.DefaultDSN(), defaultDSN...)

	db, err := sql.Open(dialect.Driver(), dsn)
	if err != nil {
		t.Fatalf("testutil.DB: open failed: %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Fatalf("testutil.DB: ping failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// PgPool creates a pgxpool connection (PostgreSQL ONLY, high performance).
// For MySQL projects, use DB() instead.
// Connection is closed via t.Cleanup.
func PgPool(t testing.TB, defaultDSN ...string) *pgxpool.Pool {
	t.Helper()
	dsn := resolveDSN(Postgres.DefaultDSN(), defaultDSN...)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("testutil.PgPool: connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Fatalf("testutil.PgPool: ping: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// resolveDSN returns DATABASE_URL env var, then the first override, then fallback.
func resolveDSN(fallback string, overrides ...string) string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	if len(overrides) > 0 && overrides[0] != "" {
		return overrides[0]
	}
	return fallback
}
