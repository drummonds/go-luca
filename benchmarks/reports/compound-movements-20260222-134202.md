# Compound Movements

**Date:** 2026-02-22 13:42:02

**Purpose:** Compare simple vs compound movement throughput on real PostgreSQL

## Database

- Source: external (BENCH_PG_DSN)
- DSN: `postgres://bench:***@localhost:15432/bench?sslmode=disable`

## SQL: Simple operation (single tx)

```sql
BEGIN;
SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements;
INSERT INTO movements (batch_id, from_account_id, to_account_id, amount,
    code, value_time, description)
  VALUES ($1, $2, $3, $4, 0, $5, 'deposit')
  RETURNING id;
SELECT
  COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1), 0)
- COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $1), 0);
COMMIT;
```

## SQL: Compound operation (single tx)

```sql
BEGIN;
SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements;
INSERT INTO movements (...) VALUES (...) RETURNING id;
SELECT ... SUM ... WHERE value_time <= eod;           -- eod balance
-- (Go: interest = balance * rate / 365)
DELETE FROM movements WHERE to_account_id=$1 AND code=1
  AND value_time >= $2 AND value_time <= $3;           -- old accrual
INSERT INTO movements (...) VALUES (...);              -- new accrual
DELETE FROM balances_live WHERE account_id=$1
  AND balance_date=$2;                                 -- old live
SELECT ... SUM ... WHERE value_time <= eod;            -- recompute
INSERT INTO balances_live (...) VALUES (...);           -- new live
COMMIT;
```

## Methods

- **Approaches:** Simple (insert + balance) vs Compound (insert + interest projection + live balance)
- **Schema:** Same as go-luca schema.go (accounts, movements, balances_live)
- **N:** Seed movements per savings account
- **M:** Number of savings accounts (plus 1 equity + 1 expense:interest)
- **Seed data:** N movements per savings account from equity, loaded via pgx CopyFrom in 10K-row batches
- **Interest:** 5% annual rate, exponent -2, computed via shopspring/decimal
- **Iteration target:** Round-robin savings accounts, unique value_time (same day, different minutes)
- **Timing:** Per-iteration wall-clock via benchutil.RunTimed
- **Warmup:** None — first iteration included
- **Transaction:** Both simple and compound wrapped in explicit pgx transactions

## Results: N=100, M=10

| Approach | N | M | Mean | TPS | P50 | P99 | Min | Max | Iters |
|----------|---|---|------|-----|-----|-----|-----|-----|-------|
| simple | 100 | 10 | 2.87ms | 348 | 2.58ms | 7.95ms | 2.27ms | 9.58ms | 100 |
| compound | 100 | 10 | 3.55ms | 281 | 3.13ms | 8.65ms | 2.92ms | 8.69ms | 100 |

## Results: N=1_000, M=100

| Approach | N | M | Mean | TPS | P50 | P99 | Min | Max | Iters |
|----------|---|---|------|-----|-----|-----|-----|-----|-------|
| simple | 1_000 | 100 | 3.08ms | 324 | 2.61ms | 7.98ms | 2.42ms | 8.77ms | 100 |
| compound | 1_000 | 100 | 3.54ms | 282 | 3.11ms | 7.83ms | 2.78ms | 8.43ms | 100 |

## Results: N=10_000, M=100

| Approach | N | M | Mean | TPS | P50 | P99 | Min | Max | Iters |
|----------|---|---|------|-----|-----|-----|-----|-----|-------|
| simple | 10_000 | 100 | 3.96ms | 252 | 3.78ms | 6.46ms | 3.24ms | 10.26ms | 100 |
| compound | 10_000 | 100 | 3.40ms | 293 | 2.97ms | 8.62ms | 2.91ms | 15.03ms | 100 |

## Purpose

When funds arrive, two approaches exist:

1. **Simple**: Record the movement, compute balances and interest on demand (or in
   batch at end of day via `RunDailyInterest`). End-of-day processing must recalculate
   everything.

2. **Compound**: At write time, in a single transaction, also pre-compute the interest
   accrual and live balance for end of day. Rollover is seamless — everything is
   already projected.

The benchmark compares throughput of both approaches on real PostgreSQL to quantify the
write-time cost of eagerly projecting interest and balances. The question: is the
compound approach fast enough to use on the hot path?

## Analysis

Results from real PostgreSQL (podman postgres:16-alpine). Placeholder — update after first run.

The compound approach runs ~7 SQL operations in a single transaction vs ~3 for simple.
The overhead comes from:
- In-transaction balance recomputation (SUM over movements)
- Interest calculation via shopspring/decimal
- Accrual upsert (DELETE + INSERT)
- Live balance upsert (DELETE + INSERT)
- A second balance recomputation after interest insertion

The benefit: no separate end-of-day batch. The projected balance and interest accrual
are always current after each movement.

## AI Summary

Compound movement (write-time projection of interest + live balance) vs simple
movement + balance query, benchmarked on real PostgreSQL.

Results placeholder — update after first run with actual TPS numbers.

The trade-off: eliminating end-of-day batch processing in exchange for extra per-write
overhead. The compound path does 7 SQL operations in a single transaction (insert
movement, compute balance, delete old accrual, insert new accrual, delete old live
balance, recompute balance, insert live balance).

