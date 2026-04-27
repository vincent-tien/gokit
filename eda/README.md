# gokit/eda

Event-Driven Architecture primitives for Go backends — broker, transactional outbox, inbox, DLQ, and event registry. Separate Go module: `gokit` core stays lightweight (no NATS / Kafka deps unless you import `gokit/eda`).

```
go get github.com/vincent-tien/gokit/eda@v0.2.0
```

Companion guide for goframe wiring: `goframe/docs/guides/09-add-event-driven-messaging.md`.

## Packages

### broker — Pub/Sub abstraction with registry-pattern factory

Pluggable backends via `Register(name, factory)`. Memory backend self-registers for tests. NATS JetStream included. Add Kafka / RabbitMQ by registering your own factory in an `init()`.

```go
pub, sub, err := broker.New(broker.Config{
    Backend: "memory",
    Options: nil,
})

// or for nats:
//   broker.New(broker.Config{Backend: "nats", Addr: "nats://localhost:4222"})

err := pub.Publish(ctx, broker.Message{Topic: "order.created", Payload: data})
err := sub.Subscribe(ctx, "order.created", handler,
    broker.WithGroup("billing"),
    broker.WithConcurrency(4),
)
```

**Backends:** `memory` · `nats` *(SubscribeOption args ignored until E15 lands — see godoc)*

**SubscribeOptions:** `WithGroup(name)` · `WithConcurrency(n)` · `WithMaxRetries(n)` · `WithAckTimeout(d)`

### event — Event metadata, envelope, dispatcher

Domain events with traceable metadata (correlation, causation, source). `Envelope` carries serialized payload + `EventMeta`.

```go
meta := event.NewMeta("order.created", 1).
    WithSource("orders").
    WithCorrelation(corrID, causID)

env, _ := event.NewEnvelope(meta, orderPayload)

// Dispatch in-process (synchronous):
disp := event.NewInProcessDispatcher()
disp.On("order.created", handler)
disp.Dispatch(ctx, env)

// Or dispatch via outbox (durable, post-commit):
od := event.NewOutboxDispatcher(writer, txAdapter, "orders")
od.Dispatch(ctx, env) // stages rows in outbox table within active tx
```

**Types:** `EventMeta{ID, Name, Version, OccurredAt, CorrelationID, CausationID, Source, Headers}` · `Envelope{Meta, Payload}` · `Dispatcher` interface

### outbox — Transactional outbox + inbox + DLQ + registry

`Writer` stages events to the `outbox` table inside a tx. `Relay` polls and publishes (adaptive 10ms..1s backoff with `FOR UPDATE SKIP LOCKED`). `Inbox` deduplicates consumer-side messages atomically with handler tx. `DLQ` retries to N max then forwards to `dlq.<topic>`. `registry` (Option-A) catalogs event names + versions for AsyncAPI export.

```go
writer := outbox.NewWriter()
writer.Store(ctx, tx, "order.created", orderID, env)

relay := outbox.NewRelay(db, pub,
    outbox.WithMinInterval(10*time.Millisecond),
    outbox.WithMaxInterval(time.Second),
    outbox.WithBatchSize(100),
)
go relay.Start(ctx)

inbox := outbox.NewInbox(db)
inbox.Process(ctx, msg.ID, func(ctx context.Context, tx *sql.Tx) error {
    return billingRepo.Apply(ctx, tx, evt)
})

dlq := outbox.NewDLQ(inbox, pub,
    outbox.WithMaxRetries(5),
    outbox.WithDLQPrefix("dlq."),
)

// Event-name catalog (opt-in, future AsyncAPI export):
outbox.MustRegister(outbox.RegisteredEvent{Name: "order.created", Version: 1, Source: "orders"})
outbox.ValidateMeta(env.Meta) // → error if (name, version) not registered
```

**Lifecycle:** `Relay.Start` blocks until ctx done; the host owns goroutine + WaitGroup. **Never** call `broker.Publish` from inside a tx — use `Writer.Store`.

### migration — DDL constants for outbox + inbox + DLQ retries

Embed in your migration tool (goose, migrate, sqlboiler, etc.).

```go
import "github.com/vincent-tien/gokit/eda/migration"

// goose Up:
db.Exec(migration.PostgresOutbox)
db.Exec(migration.PostgresInbox)
db.Exec(migration.PostgresInboxDLQRetries)

// or for MySQL:
db.Exec(migration.MySQLOutbox)
db.Exec(migration.MySQLInbox)
db.Exec(migration.MySQLInboxDLQRetries)
```

### edatest — Test helpers

In-memory broker recorder + assertion helpers. Use with the `memory` broker backend.

```go
rb := edatest.NewMemoryBroker()
err := rb.Publish(ctx, edatest.NewTestMessage("order.created", payload))

edatest.AssertEventPublished(t, rb, "order.created")
edatest.AssertInboxProcessed(t, db, msgID)
```

## Versioning

| Tag | Scope |
|-----|-------|
| `eda/v0.1.0` | Initial release — Phases 1+2 of EDA spec (E1–E14) |
| `eda/v0.2.0` | Outbox event registry (Option-A) + NATS subscribe godoc safety |

## Deferred work (future Phase 3)

- **E15** — NATS JetStream consumer-group / pull-subscribe option mapping (current `nats.go` ignores `SubscribeOption` args)
- **E16** — `distlock/` package for relay HA (multi-instance leader election)
- **E17** — broker registry refactor (cross-cutting registry API)

## License

MIT
