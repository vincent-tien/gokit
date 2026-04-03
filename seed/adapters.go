package seed

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── database/sql adapters (PRIMARY — works with ANY database) ──

type stdDBAdapter struct{ db *sql.DB }

// StdDB wraps a *sql.DB to satisfy the DB interface.
// Works with any database driver (PostgreSQL, MySQL, SQLite, etc.).
func StdDB(db *sql.DB) DB { return &stdDBAdapter{db: db} }

func (a *stdDBAdapter) Exec(ctx context.Context, query string, args ...any) error {
	_, err := a.db.ExecContext(ctx, query, args...)
	return err
}

type stdTxAdapter struct{ tx *sql.Tx }

// StdTx wraps a *sql.Tx to satisfy the DB interface.
// Use when seeds should run inside an existing transaction.
func StdTx(tx *sql.Tx) DB { return &stdTxAdapter{tx: tx} }

func (a *stdTxAdapter) Exec(ctx context.Context, query string, args ...any) error {
	_, err := a.tx.ExecContext(ctx, query, args...)
	return err
}

// ── pgx adapters (OPTIONAL — for pgxpool users, PostgreSQL only) ──

type pgxPoolAdapter struct{ pool *pgxpool.Pool }

// PgxPool wraps a *pgxpool.Pool to satisfy the DB interface.
// For PostgreSQL projects using pgx directly for high performance.
func PgxPool(pool *pgxpool.Pool) DB { return &pgxPoolAdapter{pool: pool} }

func (a *pgxPoolAdapter) Exec(ctx context.Context, query string, args ...any) error {
	_, err := a.pool.Exec(ctx, query, args...)
	return err
}

type pgxTxAdapter struct{ tx pgx.Tx }

// PgxTx wraps a pgx.Tx to satisfy the DB interface.
// Use when seeds should run inside an existing pgx transaction.
func PgxTx(tx pgx.Tx) DB { return &pgxTxAdapter{tx: tx} }

func (a *pgxTxAdapter) Exec(ctx context.Context, query string, args ...any) error {
	_, err := a.tx.Exec(ctx, query, args...)
	return err
}
