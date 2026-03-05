# SCV Generation

**Date:** 2026-03-04 21:29:01

**Purpose:** Benchmark FSCS Single Customer View file generation at various account scales

## Database

- Source: external (BENCH_PG_DSN)
- DSN: `postgres://bench:***@localhost:15432/bench?sslmode=disable`

## SQL: SCV balance query

```sql
WITH credits AS (
    SELECT to_account_id AS account_id, SUM(amount) AS total
    FROM movements GROUP BY to_account_id
),
debits AS (
    SELECT from_account_id AS account_id, SUM(amount) AS total
    FROM movements GROUP BY from_account_id
)
SELECT a.id, a.account_id, a.product, a.currency,
    COALESCE(c.total, 0) - COALESCE(d.total, 0) AS balance
FROM accounts a
LEFT JOIN credits c ON c.account_id = a.id
LEFT JOIN debits d ON d.account_id = a.id
WHERE a.account_type = 'Liability'
```

## Methods

- **Scenarios:** 1K, 10K, 100K, 1M liability accounts
- **Schema:** go-luca (accounts + movements), customers modelled via account_id field
- **Products:** Random from IAA, ISA, NA, FD1
- **Balances:** One seed movement per account (0–100K GBP random)
- **Iteration:** Full SCV generation: query → group → write C file → write D file
- **C file:** Pipe-delimited individual account records per FSCS spec
- **D file:** Pipe-delimited per-customer aggregate, compensatable capped at £85,000
- **Footer:** FSCS standard `99999999999999999999`

## Results: 1_000 accounts, 21 customers

| Type | N | M | Mean | P50 | P99 | Min | Max | Iters |
|------|---|---|------|-----|-----|-----|-----|-------|
| scv | 1_000 | 21 | 3.47ms | 3.32ms | 6.34ms | 1.71ms | 8.06ms | 50 |

## Results: 10_000 accounts, 199 customers

| Type | N | M | Mean | P50 | P99 | Min | Max | Iters |
|------|---|---|------|-----|-----|-----|-----|-------|
| scv | 10_000 | 199 | 30.82ms | 27.50ms | 48.25ms | 19.11ms | 68.78ms | 20 |

## Results: 100_000 accounts, 1_989 customers

| Type | N | M | Mean | P50 | P99 | Min | Max | Iters |
|------|---|---|------|-----|-----|-----|-----|-------|
| scv | 100_000 | 1_989 | 350.58ms | 359.45ms | 391.76ms | 291.43ms | 402.16ms | 5 |

## Results: 1_000_000 accounts, 19_677 customers

| Type | N | M | Mean | P50 | P99 | Min | Max | Iters |
|------|---|---|------|-----|-----|-----|-----|-------|
| scv | 1_000_000 | 19_677 | 2.960s | 2.960s | 2.960s | 2.960s | 2.960s | 1 |

## Purpose

FSCS requires firms to produce Single Customer View (SCV) files regularly. This benchmark
measures how fast go-luca can generate the two key CSV files from a PostgreSQL database:

- **C file**: Individual account records (one row per account, pipe-delimited)
- **D file**: Aggregate customer balances with compensatable amounts capped at £85,000

The benchmark scales from 1K to 1M accounts. Customers have 1–100 accounts each with
random balances 0–100K GBP. Products are randomly assigned from IAA, ISA, NA, and FD1.

Each iteration performs a full SCV generation: query all liability accounts with computed
balances, group by customer, write pipe-delimited C and D files with the FSCS footer.

## Analysis

## Query Optimisation: OR Join vs CTE

The initial implementation used a single OR join to compute balances:

```sql
SELECT a.id, a.account_id, a.product, a.currency,
    COALESCE(SUM(CASE WHEN m.to_account_id = a.id THEN m.amount ELSE 0 END), 0)
  - COALESCE(SUM(CASE WHEN m.from_account_id = a.id THEN m.amount ELSE 0 END), 0) AS balance
FROM accounts a
LEFT JOIN movements m ON m.to_account_id = a.id OR m.from_account_id = a.id
WHERE a.account_type = 'Liability'
GROUP BY a.id, a.account_id, a.product, a.currency
```

This was replaced with a CTE-based approach that pre-aggregates credits and debits
separately, then joins the summaries:

```sql
WITH credits AS (
    SELECT to_account_id AS account_id, SUM(amount) AS total
    FROM movements GROUP BY to_account_id
),
debits AS (
    SELECT from_account_id AS account_id, SUM(amount) AS total
    FROM movements GROUP BY from_account_id
)
SELECT a.id, a.account_id, a.product, a.currency,
    COALESCE(c.total, 0) - COALESCE(d.total, 0) AS balance
FROM accounts a
LEFT JOIN credits c ON c.account_id = a.id
LEFT JOIN debits d ON d.account_id = a.id
WHERE a.account_type = 'Liability'
```

### Results

| Accounts | OR Join (mean) | CTE (mean) | Speedup |
|----------|---------------|------------|---------|
| 1K       | 56.5ms        | 3.2ms      | ~18x    |
| 10K      | 4,598ms       | 33ms       | ~139x   |
| 100K     | did not complete | 338ms   | —       |
| 1M       | did not complete | 3.5s    | —       |

### Why

The OR join (`ON m.to_account_id = a.id OR m.from_account_id = a.id`) prevents
PostgreSQL from using a single index scan. The planner falls back to nested loops
or bitmap OR scans, giving roughly O(n * m) behaviour where n = accounts and
m = movements.

The CTE approach scans the movements table exactly twice (once per CTE), aggregates
with hash aggregation using the existing indexes, then hash-joins the two small
summary tables to accounts. This scales linearly — doubling the data roughly
doubles the time.

### Takeaway

For SCV generation at scale, always pre-aggregate movements into credit/debit
summaries before joining to accounts. The OR join pattern is only viable for
small datasets (< 1K accounts).

