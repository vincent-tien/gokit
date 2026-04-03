# gokit

Reusable Go infrastructure packages. Works with any Go backend — not framework-specific.

```
go get github.com/your-org/gokit@v0.1.0
```

## Packages

### seed — Database seeder framework

Run idempotent seed operations against any database. Framework-agnostic core with adapters for `database/sql` and `pgx`.

```go
runner := seed.NewRunner(seed.StdDB(db), seed.WithContinueOnError(true))
runner.Register(
    seed.Seed{Name: "admin-user", Run: func(ctx context.Context, db seed.DB) error {
        return db.Exec(ctx, "INSERT INTO users (id, role) VALUES (?, ?) ON CONFLICT DO NOTHING", "1", "admin")
    }},
)
result, err := runner.Run(ctx)
```

**Adapters:** `StdDB(*sql.DB)` · `StdTx(*sql.Tx)` · `PgxPool(*pgxpool.Pool)` · `PgxTx(pgx.Tx)`

**Options:** `WithLogger(l)` · `WithDryRun(true)` · `WithContinueOnError(true)`

### testutil — Test infrastructure helpers

Dialect-aware test helpers for PostgreSQL and MySQL. Set `GOKIT_DIALECT=mysql` for MySQL mode (default: postgres).

```go
func TestCreateItem(t *testing.T) {
    db := testutil.DB(t) // auto-cleanup via t.Cleanup
    testutil.Truncate(t, db, "items")

    testutil.InsertRow("items").
        Set("id", testutil.UUID(1)).
        Set("title", "Test Item").
        Set("created_at", testutil.FixedTime).
        Exec(t, db)

    // assert...
}
```

**Connections:** `DB(t)` (any dialect) · `PgPool(t)` (pgx)

**Cleanup:** `Truncate(t, db, tables...)` · `TruncatePgPool(t, pool, tables...)`

**Fixtures:** `FixedTime` · `FixedTimeAt(hours)` · `UUID(n)` · `RandomUUID()`

**Assertions:** `JSONContains(t, body, key, expected)` · `JSONEqual(t, expected, actual)`

### testutil/gintest — Gin test helpers

```go
rec := gintest.Record(engine, http.MethodPost, "/items", map[string]string{"title": "New"})
assert.Equal(t, http.StatusCreated, rec.Code)
```

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `DATABASE_URL` | Test database connection | Dialect-specific localhost |
| `GOKIT_DIALECT` | `postgres` / `mysql` / `mariadb` | `postgres` |

## Roadmap

- [x] **v0.1.0** — seed, testutil, testutil/gintest
- [ ] **v0.2.0** — cache (memory + Redis), circuit breaker
- [ ] **v0.3.0** — OpenTelemetry tracing + Prometheus metrics
- [ ] **v0.4.0** — message broker (NATS), transactional outbox

## License

MIT
