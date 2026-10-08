# Roadmap

## Done

### Ledger Interface (v0.2.5)
- `Ledger` interface decoupling operations from storage
- `SQLLedger` — full-featured database/sql backend (pglike or real postgres)
- `MemLedger` — pure Go in-memory backend (core ops, stubs for advanced)
- `NewSQLLedger(db)` for bring-your-own-driver

### Positions and contract views (v0.3.0, v0.3.1)
- Stored end-of-day projections for both accounts of a movement, rebuilt
  incrementally from the previous position, and `Project` for a day with no
  movement (gobank ADR-0002 stage 3's daily pass)
- Accrued-but-unapplied interest kept on the position as an exact fraction
- Contract views `contract_ledger_movements`, `contract_ledger_eod_positions`
  and `contract_ledger_live_positions`, money as NUMERIC in major units
- `knowledge_time` stored on every write path

### API Layer (v0.2.6)
HTTP/JSON API wrapping the Ledger interface for decoupled access.
- `api/` — server (HTTP handlers) and client (Go HTTP client implementing `Ledger`)
- Endpoints for accounts, movements, balances, options, commodities, aliases,
  customers, data points and import/export. Interest endpoints went with the
  interest logic in v0.2.26 (gobank-products owns it)
- `api.Client` implements `Ledger` — callers can swap between direct library use
  and the API transparently
- Not built: `cmd/luca-server/` binary; the server is embedded by its consumers

### Direct vs API Benchmark (v0.2.9)
- `cmd/bench-api/` — standalone benchmark using `internal/benchutil`
- Single movement TPS, balance query latency, linked movement batch throughput
- Reports and analysis in `benchmarks/ledger-backends/`

## Planned

### API Documentation (Swagger/OpenAPI)
- Generate OpenAPI spec from the API layer (see godocs project for approach)
- Serve Swagger UI alongside the API for interactive exploration
- Auto-generate Go client from the spec

### Interest Rate Precision
- Define required precision for gross and AER rates (currently stored as float64)
- Interest rates typically specified to 4dp (e.g. 4.25% = 0.0425) — what internal precision is needed for correct daily accrual?
- Analyse rounding error accumulation over long periods at different precisions
- Document precision guarantees and rounding strategy (e.g. banker's rounding)
- Consider whether `shopspring/decimal` should be used for rate storage, not just calculation

### Interest Calculation Enhancements
- Compound interest (daily, monthly, annual compounding)
- Tiered/banded interest rates (different rate above/below thresholds)
- Interest on overdrawn balances (debit interest)
- Period-end interest capitalisation

### Parameter Hierarchies
- Hierarchical parameter inheritance (e.g. default interest rate at product level, override at account level)
- Time-varying parameters (rate changes with effective dates)
- Parameter audit trail

### Accounting Hierarchies
- Configurable account aggregation trees beyond the fixed Type:Product:AccountID:Address path
- Custom reporting hierarchies (e.g. cost centres, departments)
- Balance aggregation respecting hierarchy with cross-exponent scaling

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
