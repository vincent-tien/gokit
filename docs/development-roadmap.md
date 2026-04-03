# GoKit — Development Roadmap

All 4 phases complete. Each phase ships as an independent semver tag.

---

## Phase 1: Core Infrastructure — v0.1.0 ✅

| Component | Status |
|-----------|--------|
| `seed/` — Seeder framework (DB interface, Runner, 4 adapters) | ✅ |
| `testutil/` — Dialect detection, DB helpers, truncate, fixtures, builders, assertions | ✅ |
| `testutil/gintest/` — Gin request/response test helpers | ✅ |

---

## Phase 2: Cache + Resilience — v0.2.0 ✅

| Component | Status |
|-----------|--------|
| `cache/` — Cache interface, Memory (dev), Redis (prod), GetOrLoad + singleflight | ✅ |
| `breaker/` — Circuit breaker interface + gobreaker wrapper | ✅ |

---

## Phase 3: Observability — v0.3.0 ✅

| Component | Status |
|-----------|--------|
| `otel/` — TracerProvider factory (OTLP/stdout), Prometheus metrics, RED helpers | ✅ |
| `otel/ginmw/` — Gin tracing + metrics middleware | ✅ |

---

## Phase 4: Event-Driven Architecture — v0.4.0 ✅

| Component | Status |
|-----------|--------|
| `broker/` — Publisher/Subscriber interfaces, NATS JetStream implementation | ✅ |
| `outbox/` — Transactional outbox writer, relay worker, inbox deduplication | ✅ |

---

## Dependency Map

```
v0.1.0: pgx/v5, go-sql-driver/mysql, gin, google/uuid, testify
v0.2.0: + go-redis/v9, x/sync/singleflight, sony/gobreaker/v2
v0.3.0: + otel SDK v1.43, prometheus/client_golang
v0.4.0: + nats-io/nats.go v1.50
```

Each version only adds deps for its packages. `go get gokit/seed` does NOT pull Redis/OTEL/NATS.

---

**Completed:** 2026-04-03
