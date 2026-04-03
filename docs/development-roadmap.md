# GoKit — Development Roadmap

Multi-phase infrastructure library buildout. Phases ship independently; later phases have no impact on earlier ones.

---

## Phase 1: Core Infrastructure ✅ COMPLETE

**Status:** Shipped 2026-04-03 as v0.1.0-dev
**Effort:** Complete
**Description:** Foundation packages for database seeding and test infrastructure.

| Component | Status | Details |
|-----------|--------|---------|
| `seed/` — Seeder framework | ✅ Complete | DB interface, Runner, Adapters (StdDB, StdTx, PgxPool, PgxTx) |
| `testutil/` — Test helpers | ✅ Complete | Dialect detection, DB connections, truncate, fixtures, builders |
| `testutil/gintest/` — Gin helpers | ✅ Complete | NewRequest, Record for HTTP testing |
| Project skeleton | ✅ Complete | go.mod, Makefile, linting, dependencies |

**Deliverables:**
- ✅ Seed/Runner/Adapters API
- ✅ Dialect-aware test utilities
- ✅ Test data builders with fluent API
- ✅ JSON assertion helpers
- ✅ Go 1.26 project setup

**Blockers:** None — Phase 1 ready for use by GoFrame.

---

## Phase 2: Cache + Resilience — PLANNED

**Status:** Pending
**Effort:** 3-4 days
**Trigger:** First project needs Redis/circuit breaker
**Description:** Caching layer and resilience patterns.

| Component | Status | Details |
|-----------|--------|---------|
| `cache/` — Cache abstraction | 🔲 Planned | Memory impl (dev), Redis impl (go-redis), Singleflight wrapper |
| `breaker/` — Circuit breaker | 🔲 Planned | Interface + gobreaker wrapper, fallback support |

**Dependencies:** Phase 1 complete
**Integration point:** GoFrame cache service layer

---

## Phase 3: Observability — PLANNED

**Status:** Pending
**Effort:** 2-3 days
**Trigger:** First production service needs tracing
**Description:** OpenTelemetry integration and middleware.

| Component | Status | Details |
|-----------|--------|---------|
| `otel/` — OTEL setup | 🔲 Planned | Tracer factory, metrics factory (Prometheus) |
| `otel/ginmw/` — Gin tracing | 🔲 Planned | Request/span middleware for distributed tracing |

**Dependencies:** Phase 1 complete
**Integration point:** GoFrame Gin middleware stack

---

## Phase 4: Event-Driven Architecture — PLANNED

**Status:** Pending
**Effort:** 4-5 days
**Trigger:** First async messaging requirement (sagas, event sourcing)
**Description:** Transactional messaging and broker abstraction.

| Component | Status | Details |
|-----------|--------|---------|
| `outbox/` — Outbox pattern | 🔲 Planned | Write to outbox in same tx, relay worker, inbox dedup |
| `broker/` — Message broker | 🔲 Planned | Publisher/Subscriber interfaces, NATS implementation |

**Dependencies:** Phase 1 + Phase 2 (circuit breaker for reliability)
**Integration point:** GoFrame event service layer

---

## Success Metrics

| Phase | Target | Indicator |
|-------|--------|-----------|
| Phase 1 | ✅ Core ready | GoFrame compiles without gokit compilation errors |
| Phase 2 | 🔲 Cache operational | First Redis project deploys, 0 cache coherency bugs in prod |
| Phase 3 | 🔲 Observability complete | 100% request spans traced, <50ms overhead per request |
| Phase 4 | 🔲 EDA stable | All async workflows handle failures + retries, 0 message loss |

---

## Timeline

- **Week 1 (Mar 31 - Apr 6):** Phase 1 shipment + GoFrame integration
- **Week 2-3 (Apr 7 - Apr 20):** Phase 2 (if Redis project incoming)
- **Week 4-5 (Apr 21 - May 4):** Phase 3 (if observability needed)
- **Week 6-7 (May 5 - May 18):** Phase 4 (if EDA needed)

**Principle:** Ship when needed, not speculatively. Phases are independent.

---

**Last Updated:** 2026-04-03
**Next Review:** After Phase 1 GoFrame integration complete
