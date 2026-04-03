# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

GoKit is a reusable Go infrastructure library (`github.com/your-org/gokit`). It provides shared, framework-agnostic packages for any Go backend. GoFrame imports GoKit — GoKit **never** imports GoFrame.

Go 1.26 (toolchain go1.26.1). MIT license.

## Build & Test Commands

```bash
make test          # go test -race -count=1 ./...
make test-v        # go test -race -count=1 -v ./...
make lint          # golangci-lint run ./...
make fmt           # gofmt -s -w . && goimports -w .
make verify        # fmt lint test (full pre-push check)

# Single package test
go test -race -count=1 -v ./seed/...
go test -race -count=1 -v ./testutil/...

# Integration tests (need real DB)
DATABASE_URL="postgres://user:pass@localhost:5432/gokit_test" go test -race ./...

# MySQL mode
GOKIT_DIALECT=mysql DATABASE_URL="user:pass@tcp(localhost:3306)/gokit_test?parseTime=true" go test -race ./...

# Security scan
govulncheck ./...
```

## Architecture

### Dependency Direction (non-negotiable)
```
GoFrame ──imports──> gokit
gokit   ──imports──> stdlib + pgx + (optional: go-redis, otel-sdk, gobreaker, nats)
gokit   ──NEVER imports──> GoFrame or any business logic
```

### Package Structure
```
seed/                    Database seeder framework (core types + Runner)
seed/adapters.go         StdDB, StdTx (database/sql), PgxPool, PgxTx (pgx) — adapters for seed.DB interface
testutil/                Test infrastructure: DB connections, Truncate, ExecSQL, fixtures, builders
testutil/gintest/        Gin-specific test helpers (NewRequest, Record) — only package that imports Gin
cache/                   Cache interface + Memory (dev) + Redis (prod) + singleflight  [Phase 2]
breaker/                 Circuit breaker interface + gobreaker wrapper                  [Phase 2]
otel/                    OpenTelemetry tracer + Prometheus metrics factories             [Phase 3]
otel/ginmw/              Gin middleware for tracing/metrics                              [Phase 3]
broker/                  Publisher/Subscriber interfaces + NATS JetStream                [Phase 4]
outbox/                  Transactional outbox pattern (Writer, Relay, Inbox)             [Phase 4]
```

### Key Design Patterns

- **Adapter pattern**: `seed.DB` interface has one method (`Exec`). Adapters wrap `*sql.DB`, `*sql.Tx`, `*pgxpool.Pool`, `pgx.Tx`.
- **Dialect awareness**: `testutil.DialectFromEnv()` returns `Postgres` or `MySQL`. SQL differences (TRUNCATE syntax, foreign key handling) are handled per-dialect.
- **Functional options**: `seed.NewRunner(db, WithLogger(l), WithDryRun(true))`
- **Fluent builder**: `testutil.InsertRow("items").Set("id", testutil.UUID(1)).Set("title", "Test").Exec(t, db)`
- **Deterministic fixtures**: `testutil.UUID(1)` always returns `00000000-0000-0000-0000-000000000001`. `testutil.FixedTime` is always `2026-01-01T12:00:00Z`.
- **Phased delivery**: Only Phase 1 deps ship in v0.1.0. `go get gokit/seed` does NOT pull Redis/OTEL/NATS.

### Anti-Patterns to Avoid

1. Never import GoFrame from gokit
2. Never add business logic — infrastructure only
3. Never put framework-specific code in core packages — use sub-packages (`gintest/`)
4. Never use PostgreSQL-specific SQL in core packages — use `?` placeholders, standard SQL
5. Never hardcode PostgreSQL as only engine — always support `database/sql` interface
6. `seed.go` imports ONLY stdlib (`context`, `fmt`, `time`) — zero database driver imports
7. Each package independently importable — no "god packages"

## Implementation Plan

Detailed spec: `specs/GOKIT_IMPLEMENTATION_PLAN.md`

Phases are delivered incrementally (build only when first project needs it):
- **Phase 1** (v0.1.0): `seed/` + `testutil/` — GoFrame depends on this, ship first
- **Phase 2** (v0.2.0): `cache/` + `breaker/` — when projects need Redis/resilience
- **Phase 3** (v0.3.0): `otel/` — when projects need observability
- **Phase 4** (v0.4.0): `broker/` + `outbox/` — when projects need event-driven architecture

## Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `DATABASE_URL` | Test database connection string | Dialect-specific localhost DSN |
| `GOKIT_DIALECT` | Database engine: `postgres`, `mysql`, `mariadb` | `postgres` |
