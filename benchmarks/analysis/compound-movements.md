# Compound Movements Benchmark

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

## Summary

Compound movement (write-time projection of interest + live balance) vs simple
movement + balance query, benchmarked on real PostgreSQL.

Results placeholder — update after first run with actual TPS numbers.

The trade-off: eliminating end-of-day batch processing in exchange for extra per-write
overhead. The compound path does 7 SQL operations in a single transaction (insert
movement, compute balance, delete old accrual, insert new accrual, delete old live
balance, recompute balance, insert live balance).
