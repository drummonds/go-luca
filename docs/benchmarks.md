# Benchmarks

## Running

```bash
task benchmark          # Standard Go benchmarks (pglike/SQLite, :memory:)
task testBenchmark      # Standalone PG benchmarks (starts/stops Podman container)
```

## Structure

```
benchmark_test.go           # Standard Go benchmarks (in-process, :memory:)
cmd/bench-*/                # Standalone programs for real PostgreSQL
benchmarks/
  analysis/                 # Narrative analysis per topic (purpose, findings, AI summary)
  reports/                  # Timestamped raw reports with data tables and SQL queries
```

Each benchmark topic has a single `<topic>.md` file containing purpose, analysis, and summary sections.

Reports in `benchmarks/reports/` include full SQL queries, schema, raw output, and results tables (TPS).

## Writing Benchmarks

- Use `:memory:` for in-process benchmarks (no disk I/O noise)
- `b.ResetTimer()` after setup
- Sub-benchmarks (`b.Run(...)`) to vary parameters
- `l.EnsureInterestAccounts()` during setup for interest-related benchmarks
- Standalone PG benchmarks go in `cmd/bench-*`, wired into `task testBenchmark`

## Benchmark Index

### In-process (pglike/SQLite)

| Benchmark | What it measures | TPS |
|-----------|-----------------|----:|
| `BenchmarkRecordMovement` | Single movement insert | ~2,318 |
| `BenchmarkBalanceQuery` | Balance query vs movement count (100/1k/10k) | — |
| `BenchmarkSimpleMovementAndBalance` | Movement + Balance per op (baseline) | ~1,260 |
| `BenchmarkCompoundMovementWithProjections` | Compound: movement + interest + live balance | ~586 |
| `BenchmarkInterestNAccounts` | RunDailyInterest scaling (10/100/1k/10k accounts) | — |

### Standalone (real PostgreSQL)

| Benchmark | What it measures | Analysis |
|-----------|-----------------|----------|
| `bench-balance-types` | BIGINT vs DOUBLE vs NUMERIC for point-in-time lookup | [balance-types](benchmarks/analysis/balance-types.html) |
| `bench-compound-movements` | Simple vs compound movement throughput | [compound-movements](benchmarks/analysis/compound-movements.html) |
| `bench-scv` | Single Customer View generation | [scv](benchmarks/analysis/scv.html) |
| `bench-api` | Direct library calls vs HTTP API | [ledger-backends](benchmarks/analysis/ledger-backends.html) |
