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

Each benchmark topic produces three analysis files:
- `<topic>-purpose.md` — what scenario it simulates and why
- `<topic>-analysis.md` — conclusions and recommendations
- `<topic>-ai-summary.md` — compact summary

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
| `bench-balance-types` | BIGINT vs DOUBLE vs NUMERIC for point-in-time lookup | `benchmarks/analysis/balance-types-*` |
| `bench-compound-movements` | Simple vs compound movement throughput | `benchmarks/analysis/compound-movements-*` |

## Analysis Reports

| Topic | Question | Finding |
|-------|----------|---------|
| [Balance types](../benchmarks/analysis/balance-types-analysis.md) | Which PG type for balance storage? | BIGINT — sub-ms lookups at 36M rows, all types equivalent |
| [Compound movements](../benchmarks/analysis/compound-movements-analysis.md) | Is write-time projection fast enough? | ~586 TPS, 2.2x overhead vs simple — eliminates batch processing |
