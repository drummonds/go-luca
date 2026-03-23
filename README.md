# go-luca

A Go library for movement-based double-entry bookkeeping, inspired by [Luca Pacioli](https://www.bytestone.uk/afp/historical-accounting/pacioli/) and [TigerBeetle](https://tigerbeetle.com/).

## Overview

go-luca treats **cash movements** as the fundamental accounting record, not posting legs. This follows the principle described at [bytestone.uk/afp/movements](https://www.bytestone.uk/afp/movements/). Movements are grouped into batches that occur at a single point in time, within transactions that span a period.

Key design choices:

- **Amounts** are `int64` in smallest currency unit (e.g. pence), with an exponent per account. No floating-point anywhere in core accounting.
- **Same-exponent rule** per ledger — movements only between accounts with matching exponents. Cross-exponent transfers are explicit currency conversions.
- **Bitemporal** — each event has a value time and an optional knowledge time.
- **Pluggable backends** — `SQLLedger` (pglike/SQLite or PostgreSQL), `MemLedger` (in-memory), or HTTP/JSON API client. All implement the `Ledger` interface.

## Architecture

```
Ledger (interface)
  ├── SQLLedger    — database/sql backend (pglike or postgres)
  ├── MemLedger    — pure Go in-memory
  └── api.Client   — HTTP/JSON client (implements Ledger)
```

### Account hierarchy

Accounts follow a colon-separated path: `Type:Product:AccountID:Address`. Balances can be queried at any level of the hierarchy.

### Interest

Daily interest accrual using actual/365. See [research/interest/](research/interest/) for the design rationale covering AER, day-count conventions, rounding strategies, and the discrete-interest formula.

## Getting started

```go
import "codeberg.org/hum3/go-luca"

db, _ := sql.Open("pglike", "file:ledger.db")
ledger, _ := luca.NewSQLLedger(db)

acct, _ := ledger.CreateAccount("Asset:Bank:Current", "GBP", -2, 0)
```

See `cmd/example/main.go` for a working demo.

## Project structure

| Path | Description |
|---|---|
| `luca.go` | Core types: Account, Movement, Ledger interface |
| `db.go` | SQLLedger CRUD + same-exponent validation |
| `balance.go` | Balance queries (point-in-time, by path, daily) |
| `interest.go` | Daily interest engine |
| `decimal.go` | int64/decimal conversion helpers |
| `schema.go` | DDL schema creation |
| `api/` | HTTP/JSON server and client |
| `cmd/` | Binaries (server, example, benchmarks) |
| `benchmarks/` | Benchmark results and analysis |
| `research/` | Design research (numeric formats, interest) |

## References

- [Plain text syntax](./TEXT_SYNTAX.md)

## Links

| | |
|---|---|
| Documentation | https://h3-go-luca.statichost.page/ |
| Source (Codeberg) | https://codeberg.org/hum3/go-luca |
| Mirror (GitHub) | https://github.com/drummonds/go-luca |
| Docs repo | https://codeberg.org/hum3/go-luca-docs |
