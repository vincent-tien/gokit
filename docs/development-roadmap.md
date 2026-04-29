# GoKit — Development Roadmap

All 5 phases complete. Each phase ships as an independent semver tag.

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

## Phase 5: Microservice Support — v0.5.0 ✅

| Component | Status |
|-----------|--------|
| `grpcclient/` — gRPC Dial factory (TLS, keepalive, timeout, interceptor chain) | ✅ |
| `grpcclient/interceptor/` — RequestID, Logging, Retry, CircuitBreaker | ✅ |
| `grpcclient/health.go` — Standard grpc.health.v1 health check | ✅ |
| `discovery/` — Resolver interface, Static (dev), Consul (prod) | ✅ |

---

## Phase 6: EDA Submodule Split — gokit v0.6.0 + eda/v0.1.0 ✅

Broker, outbox, inbox, and DLQ moved into a separate Go module at `eda/` so projects opt-in to messaging deps. Core stays lightweight — `nats-io/nats.go` is no longer pulled by `gokit` itself.

| Component | Status |
|-----------|--------|
| `eda/` — separate Go module (`github.com/vincent-tien/gokit/eda`) | ✅ |
| `eda/broker/` — registry-pattern Pub/Sub (memory + nats backends) | ✅ |
| `eda/event/` — EventMeta + Envelope + InProcessDispatcher + OutboxDispatcher (TxFromContext interface) | ✅ |
| `eda/outbox/` — Writer + Inbox + adaptive Relay (10ms..1s) + DLQ with separate retry table | ✅ |
| `eda/migration/` — Postgres + MySQL DDL constants | ✅ |
| `eda/edatest/` — RecordingBroker + AssertEventPublished / AssertInboxProcessed + fixtures | ✅ |
| `broker/` + `outbox/` removed from gokit core | ✅ |

---

## Phase 7: Registry-Pattern Factories — gokit v0.7.0 + eda/v0.2.0 ✅

Audit-driven completion of the registry-pattern story across `cache/` and `discovery/`, plus an event-name catalog in the eda submodule.

| Component | Status |
|-----------|--------|
| `cache/factory.go` — registry pattern; memory + redis self-register via init() | ✅ |
| `discovery/factory.go` — registry pattern; static + consul self-register via init() | ✅ |
| `eda/outbox/registry.go` — AsyncAPI catalog (name + version + source) for event-schema export | ✅ |
| `eda/broker/nats.go` — public godoc warning that SubscribeOption args ignored until E15 | ✅ |
| `eda/README.md` (eda/v0.2.1) — submodule README | ✅ |

Existing direct constructors (`NewMemory`, `NewRedis`, `Static`, `Consul`) unchanged for backward compatibility.

---

## Phase 8: RabbitMQ Backend — eda/v0.3.0 ✅

Production-ready AMQP 0.9.1 backend for the broker registry. Spec task E19 from the EDA implementation plan. Default queue type is quorum (replicated, durable, x-delivery-count tracking) — matches the resilience baseline of the NATS JetStream backend.

| Component | Status |
|-----------|--------|
| `eda/broker/rabbitmq.go` — factory, init() registration, options parser, header/delivery codecs, retryable-error classifier, panic-safe handler | ✅ |
| `eda/broker/rabbitmq_publisher.go` — mutex-guarded channel, publisher confirms, persistent delivery, exchange-declare cache, reconnect-with-backoff (5 attempts, linear) | ✅ |
| `eda/broker/rabbitmq_subscriber.go` — per-Subscribe channel, quorum queue, QoS=Concurrency, worker pool, manual ack/nack/reject, native-DLX option, drain-on-Close (30s) | ✅ |
| `eda/broker/rabbitmq_test.go` — 54 unit tests (interface compliance, options, codecs, validation, registry routing) | ✅ |
| `eda/broker/rabbitmq_integration_test.go` — 7 e2e tests (`//go:build integration`): pub/sub, round-robin, fan-out, nack-requeue, concurrency, headers, graceful close | ✅ |
| `eda/docker-compose.test.yml` — `rabbitmq:3.13-management-alpine` with healthcheck | ✅ |
| `Makefile` — `test-rabbitmq{,-up,-down}` targets | ✅ |
| `eda/README.md` — RabbitMQ entry + Config.Options table | ✅ |

**Config.Options:** `vhost`, `heartbeat`, `exchange_type`, `binding_key`, `prefetch_global`, `native_dlq`, `dlq_prefix`, `durable`. Defaults match production conventions (topic exchange, # binding, durable=true, native_dlq=false — favour app-level inbox/dlq for portability).

Two AMQP connections per `broker.New(rabbitmq)` call — publisher + subscriber isolated so a stuck consumer cannot stall publishes.

---

## Dependency Map

```
v0.1.0: pgx/v5, go-sql-driver/mysql, gin, google/uuid, testify
v0.2.0: + go-redis/v9, x/sync/singleflight, sony/gobreaker/v2
v0.3.0: + otel SDK v1.43, prometheus/client_golang
v0.4.0: + nats-io/nats.go v1.50
v0.5.0: + google.golang.org/grpc, hashicorp/consul/api
v0.6.0: − nats-io/nats.go v1.50 (moved to eda/go.mod)
v0.7.0: no dep changes
eda/v0.3.0: + github.com/rabbitmq/amqp091-go v1.10.0 (eda submodule only)
```

Each version only adds deps for its packages. `go get gokit/seed` does NOT pull Redis/OTEL/NATS/gRPC.

---

**Completed:** 2026-04-27
