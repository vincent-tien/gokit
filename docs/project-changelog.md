# GoKit — Project Changelog

All notable changes to GoKit are recorded here, following semantic versioning.

---

## [0.1.0-dev] — 2026-04-03

### Added — Phase 1: Core Infrastructure

**`seed/` package — Database Seeder Framework**
- `Seed` type: named, idempotent data operations
- `Runner` type: sequential seed executor with dry-run and error handling
- Adapters: `StdDB`, `StdTx` (sql.DB/sql.Tx), `PgxPool`, `PgxTx` (pgx native)
- Options: `WithLogger()`, `WithDryRun()`, `WithContinueOnError()`

**`testutil/` package — Test Infrastructure Helpers**
- Dialect detection: `Dialect` enum (PostgreSQL, MySQL), `DialectFromEnv()`
- Database connections: `DB()` (universal *sql.DB), `PgPool()` (pgx-specific)
- Table truncation: `Truncate()` (dialect-aware), `TruncatePgPool()`
- SQL execution: `ExecSQL()`, `ExecSQLPgPool()`
- Fixtures: `FixedTime`, `FixedTimeAt()`, `UUID()`, `RandomUUID()`
- Test data builders: `Row`, `Insert()`, `Values()` with fluent API
- JSON assertions: `JSONContains()`, `JSONEqual()`

**`testutil/gintest/` package — Gin-Specific Test Helpers**
- Request builders: `NewRequest()` (GET, POST, PUT, DELETE)
- Response recording: `Record()` with status, headers, body access

### Dependencies
- `github.com/jackc/pgx/v5` v5.7.2 — PostgreSQL driver (native)
- `github.com/go-sql-driver/mysql` v1.8.1 — MySQL/MariaDB driver
- `github.com/gin-gonic/gin` v1.10.0 — Web framework (testutil/gintest only)
- `github.com/google/uuid` v1.6.0 — UUID generation
- `github.com/stretchr/testify` v1.9.0 — Test assertions

### Infrastructure
- Go 1.26+ (toolchain go1.26.1)
- Module: `github.com/vincent-tien/gokit`
- License: MIT
- Build: Makefile with test, lint, fmt, verify targets
- Linting: golangci-lint configuration

### Documentation
- Phase 1 (core seed + testutil) shipped as foundation for GoFrame integration

---

## Upcoming Releases

**[0.2.0]** — Phase 2: Cache + Resilience
- `cache/` — Cache abstraction (Memory, Redis, Singleflight)
- `breaker/` — Circuit breaker integration

**[0.3.0]** — Phase 3: Observability
- `otel/` — OpenTelemetry setup (tracer, metrics)
- `otel/ginmw/` — Gin tracing middleware

**[0.4.0]** — Phase 4: Event-Driven Architecture
- `outbox/` — Transactional outbox pattern
- `broker/` — Message broker abstraction (NATS)

---

## Release Policy

- **Stable:** v0.x.0 for core infrastructure
- **Experimental:** v0.x.0-alpha, v0.x.0-beta before stability
- **Compatibility:** Semantic versioning; v0.x breaking changes documented

---

**Generated:** 2026-04-03
