# GoKit — Project Changelog

All notable changes to GoKit, following semantic versioning.

---

## [0.4.0] — 2026-04-03

### Added — Phase 4: Event-Driven Architecture

- `broker/` — Message broker abstraction: `Message`, `Publisher`, `Subscriber` interfaces
- `broker/nats.go` — NATS JetStream implementation with message ID dedup and key propagation
- `outbox/` — Transactional outbox: `Writer.Store()` inserts events in same DB transaction
- `outbox/relay.go` — `Relay` polls outbox, publishes to broker, deletes on success (FOR UPDATE SKIP LOCKED)
- `outbox/inbox.go` — `Inbox.Process()` deduplicates via INSERT ON CONFLICT

### Dependencies added
- `github.com/nats-io/nats.go` v1.50.0

---

## [0.3.0] — 2026-04-03

### Added — Phase 3: Observability

- `otel/tracer.go` — `NewTracer()` TracerProvider factory (OTLP gRPC prod, stdout dev)
- `otel/metrics.go` — `NewMetrics()` Prometheus exporter + `NewREDMetrics()` Rate/Error/Duration helpers
- `otel/ginmw/` — `Trace()` per-request span middleware, `Metrics()` RED recording middleware

### Dependencies added
- `go.opentelemetry.io/otel` SDK v1.43.0
- `github.com/prometheus/client_golang` v1.23.2

---

## [0.2.0] — 2026-04-03

### Added — Phase 2: Cache + Resilience

- `cache/cache.go` — `Cache` interface (Get/Set/Delete/Exists), `ErrNotFound` sentinel
- `cache/memory.go` — In-memory cache with TTL, copy-safe, RWMutex (dev/test)
- `cache/redis.go` — Redis-backed cache via go-redis v9
- `cache/cache.go` — `GetOrLoad()` cache-aside with singleflight thundering herd prevention
- `breaker/breaker.go` — `Breaker` interface + gobreaker wrapper, functional options, `ErrOpen` sentinel

### Dependencies added
- `github.com/redis/go-redis/v9` v9.18.0
- `golang.org/x/sync` (singleflight)
- `github.com/sony/gobreaker/v2` v2.4.0

---

## [0.1.0] — 2026-04-03

### Added — Phase 1: Core Infrastructure

- `seed/seed.go` — `DB` interface, `Seed`, `Runner`, `Result`, `SeedError` (with Unwrap)
- `seed/adapters.go` — `StdDB`, `StdTx`, `PgxPool`, `PgxTx` adapters
- `testutil/` — Dialect detection, DB/PgPool connections, Truncate, ExecSQL, ExecPgPool
- `testutil/fixture.go` — `FixedTime`, `FixedTimeAt()`, `UUID(n)`, `RandomUUID()`
- `testutil/builder.go` — `InsertRow` fluent builder, `InsertRows` batch
- `testutil/assert.go` — `JSONContains`, `JSONEqual`
- `testutil/gintest/` — `NewRequest`, `Record` for Gin handler testing

### Dependencies
- `github.com/jackc/pgx/v5`, `github.com/go-sql-driver/mysql`, `github.com/gin-gonic/gin`
- `github.com/google/uuid`, `github.com/stretchr/testify`

### Infrastructure
- Go 1.26, module `github.com/vincent-tien/gokit`, MIT license

---

**Last updated:** 2026-04-03
