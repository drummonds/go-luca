# Roadmap

## Done

### Ledger Interface (v0.2.5)
- `Ledger` interface decoupling operations from storage
- `SQLLedger` — full-featured database/sql backend (pglike or real postgres)
- `MemLedger` — pure Go in-memory backend (core ops, stubs for advanced)
- `NewSQLLedger(db)` for bring-your-own-driver

## In Progress

### API Layer
HTTP/JSON API wrapping the Ledger interface for decoupled access.

**Packages:**
- `api/` — server (HTTP handlers) and client (Go HTTP client implementing `Ledger`)
- `cmd/luca-server/` — binary serving the API

**Endpoints** (all JSON, POST for writes, GET for reads):
- `/accounts` — create, get, list
- `/movements` — record, record-linked, list
- `/balances` — balance, balance-at, balance-by-path, daily-balances, live-balance
- `/interest` — ensure-accounts, calculate, run-daily, run-period
- `/import`, `/export` — journal import/export

**Key design:** `api.Client` implements `Ledger` — callers can swap between direct library use and API transparently.

### Direct vs API Benchmark
Compare performance of direct method calls against the HTTP/JSON API layer.

- `cmd/bench-api/` — standalone benchmark using `internal/benchutil`
- Measures: single movement TPS, balance query latency, linked movement batch throughput
- Reports to `benchmarks/reports/` and analysis in `benchmarks/analysis/`
- Appears in docs documentation table

## Planned

### Ledger Backend Variants
- **Compact MemLedger** — columnar storage for read-heavy analytics
- **Large MemLedger** — sharded maps for high-concurrency writes
- **pgx backend** — high-performance PostgreSQL via pgx/v5 (bypassing database/sql)

### Bitemporal Queries
- Knowledge-time-aware balance methods (as-of / as-known-at)
- Full bitemporal reporting: value_time x knowledge_time

### Visualization
- `cmd/luca-ui/` — lofigui HTML/CSS UI for API endpoints
- Account browser, movement timeline, balance charts
- Served alongside or independently of the API

### Group Accounting
- Aggregate smaller entities into larger ones (e.g. branches into bank)
- Cross-entity reporting with currency conversion
