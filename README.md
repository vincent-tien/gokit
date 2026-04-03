# gokit

Reusable Go infrastructure packages. Works with any Go backend — not framework-specific.

```
go get github.com/vincent-tien/gokit@v0.4.0
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

### cache — Cache abstraction

In-memory (dev/test) and Redis (production) implementations with cache-aside + singleflight.

```go
c := cache.NewMemory()                    // dev/test
c := cache.NewRedis(redisClient)          // production

val, err := cache.GetOrLoad(ctx, c, "user:123", 5*time.Minute, func(ctx context.Context) ([]byte, error) {
    return json.Marshal(repo.GetUser(ctx, "123"))
})
```

**Interface:** `Get` · `Set` · `Delete` · `Exists`

**Implementations:** `NewMemory()` · `NewRedis(client)`

**Helper:** `GetOrLoad(ctx, cache, key, ttl, loader)` — cache-aside with singleflight dedup

### breaker — Circuit breaker

```go
b := breaker.New("payments", breaker.WithTimeout(30*time.Second))
err := b.Execute(ctx, func() error {
    return paymentClient.Charge(ctx, amount)
})
```

**Options:** `WithMaxRequests(n)` · `WithInterval(d)` · `WithTimeout(d)` · `WithReadyToTrip(fn)`

**States:** `StateClosed` · `StateOpen` · `StateHalfOpen`

### otel — OpenTelemetry + Prometheus

```go
tp, _ := otel.NewTracer(otel.TracerConfig{ServiceName: "my-api", Env: "prod", Endpoint: "localhost:4317"})
defer tp.Shutdown(ctx)

handler, mp, _ := otel.NewMetrics(otel.MetricsConfig{})
red, _ := otel.NewREDMetrics(mp.Meter("http"), "http")
http.Handle("/metrics", handler)
```

### otel/ginmw — Gin observability middleware

```go
engine.Use(ginmw.Trace("my-api"))
engine.Use(ginmw.Metrics(red))
```

### broker — Message broker abstraction

```go
pub := broker.NewNATSPublisher(js)
pub.Publish(ctx, broker.Message{Topic: "orders.created", Key: "order-123", Payload: data})

sub := broker.NewNATSSubscriber(js)
sub.Subscribe(ctx, "orders", func(ctx context.Context, msg broker.Message) error {
    return processOrder(ctx, msg.Payload)
})
```

### outbox — Transactional outbox pattern

```go
writer := outbox.NewWriter(db)
tx, _ := db.BeginTx(ctx, nil)
writer.Store(ctx, tx, outbox.Event{Topic: "orders.created", Key: orderID, Payload: data})
tx.Commit()

relay := outbox.NewRelay(db, publisher, outbox.WithInterval(time.Second))
go relay.Start(ctx)

inbox := outbox.NewInbox(db)
inbox.Process(ctx, msgID, func() error { return handleOrder() })
```

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `DATABASE_URL` | Test database connection | Dialect-specific localhost |
| `GOKIT_DIALECT` | `postgres` / `mysql` / `mariadb` | `postgres` |

## Versions

- [x] **v0.1.0** — seed, testutil, testutil/gintest
- [x] **v0.2.0** — cache (memory + Redis), circuit breaker
- [x] **v0.3.0** — OpenTelemetry tracing + Prometheus metrics
- [x] **v0.4.0** — message broker (NATS), transactional outbox

## License

MIT
