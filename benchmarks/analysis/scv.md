# SCV Benchmark

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

### Query Optimisation: OR Join vs CTE

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
