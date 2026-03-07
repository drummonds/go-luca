# Ledger Backends

**Date:** 2026-03-05 00:47:25

**Purpose:** Compare Ledger performance across backends: MemLedger, SQLLedger (pglike), and HTTP/JSON API

## Methods

- **mem:** Pure Go in-memory MemLedger
- **sql:** SQLLedger with pglike/SQLite :memory:
- **api:** api.Client → httptest.Server → api.Server → SQLLedger :memory:
- **N:** Number of movements (pre-loaded for balance queries)
- **M:** Number of accounts
- Each scenario creates a fresh ledger to avoid cross-contamination
- Warmup: None — first iteration included

## Results: RecordMovement (small, M=10)

| Approach | N | M | Mean | TPS | P50 | P99 | Min | Max | Iters |
|----------|---|---|------|-----|-----|-----|-----|-----|-------|
| mem | 0 | 10 | 0.3us | 2_949_852 | 0.1us | 0.1us | 0.1us | 2.4us | 10 |
| sql | 0 | 10 | 553.4us | 1_807 | 278.4us | 538.1us | 225.2us | 2.47ms | 10 |
| api | 0 | 10 | 1.11ms | 904 | 840.0us | 1.86ms | 443.4us | 2.07ms | 10 |

## Results: RecordMovement (medium, M=100)

| Approach | N | M | Mean | TPS | P50 | P99 | Min | Max | Iters |
|----------|---|---|------|-----|-----|-----|-----|-----|-------|
| mem | 0 | 100 | 0.3us | 3_003_003 | 0.2us | 0.4us | 0.1us | 1.5us | 10 |
| sql | 0 | 100 | 2.30ms | 435 | 1.95ms | 2.56ms | 1.43ms | 5.33ms | 10 |
| api | 0 | 100 | 4.00ms | 249 | 3.17ms | 5.93ms | 945.2us | 8.90ms | 10 |

## Results: RecordMovement (large, M=1_000)

| Approach | N | M | Mean | TPS | P50 | P99 | Min | Max | Iters |
|----------|---|---|------|-----|-----|-----|-----|-----|-------|
| mem | 0 | 1_000 | 0.6us | 1_700_680 | 0.3us | 1.1us | 0.2us | 2.3us | 10 |
| sql | 0 | 1_000 | 467.5us | 2_139 | 344.8us | 1.01ms | 264.4us | 1.03ms | 10 |
| api | 0 | 1_000 | 1.46ms | 685 | 1.17ms | 2.47ms | 489.8us | 3.53ms | 10 |

## Results: Balance query (small, N=100, M=10)

| Type | N | M | Mean | P50 | P99 | Min | Max | Iters |
|------|---|---|------|-----|-----|-----|-----|-------|
| mem | 100 | 10 | 0.2us | 0.2us | 0.3us | 0.2us | 0.8us | 10 |
| sql | 100 | 10 | 240.4us | 157.3us | 184.4us | 144.6us | 960.7us | 10 |
| api | 100 | 10 | 395.0us | 310.8us | 358.9us | 298.1us | 1.04ms | 10 |

## Results: Balance query (medium, N=1_000, M=100)

| Type | N | M | Mean | P50 | P99 | Min | Max | Iters |
|------|---|---|------|-----|-----|-----|-----|-------|
| mem | 1_000 | 100 | 1.4us | 1.3us | 1.3us | 1.3us | 1.9us | 10 |
| sql | 1_000 | 100 | 441.0us | 365.3us | 473.3us | 339.1us | 916.1us | 10 |
| api | 1_000 | 100 | 555.1us | 475.5us | 656.5us | 435.3us | 1.11ms | 10 |

## Results: Balance query (large, N=10_000, M=1_000)

| Type | N | M | Mean | P50 | P99 | Min | Max | Iters |
|------|---|---|------|-----|-----|-----|-----|-------|
| mem | 10_000 | 1_000 | 15.6us | 11.8us | 17.4us | 11.7us | 42.9us | 10 |
| sql | 10_000 | 1_000 | 7.84ms | 5.99ms | 13.88ms | 2.32ms | 14.56ms | 10 |
| api | 10_000 | 1_000 | 5.79ms | 3.22ms | 14.95ms | 2.35ms | 17.01ms | 10 |

## Purpose

Compare Ledger performance across backends: MemLedger (pure Go), SQLLedger (pglike/SQLite),
and the HTTP/JSON API layer. This quantifies the cost of each abstraction level so users can
make informed choices between embedding go-luca as a library or running it as a service.

Key questions:
- How fast is MemLedger vs SQLLedger for core operations?
- What is the per-call overhead of the HTTP/JSON API layer?
- How does overhead scale with pre-loaded data volume?

## Analysis

_Run `task bench:api` to generate results, then update this file with analysis._

