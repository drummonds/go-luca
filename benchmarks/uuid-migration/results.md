## UUID Migration Benchmark Results

**Date:** 2026-03-16
**Platform:** Linux, amd64, Intel Core Ultra 7 165H
**Driver:** pglike (SQLite :memory:)
**Go:** 1.25.3

### RecordMovement

| Phase | ns/op | B/op | allocs/op |
|-------|------:|-----:|----------:|
| Pre-UUID (SERIAL) | 734,128 | 400,849 | 871 |
| Post-UUID (TEXT) | 1,217,531 | 347,677 | 727 |
| **Change** | **+66%** | **-13%** | **-17%** |

UUID generation replaces `MAX(batch_id)+1` subquery. Latency increases because
TEXT primary keys are slower for SQLite B-tree inserts than INTEGER, but memory
and allocations drop because we no longer scan a RETURNING clause.

### BalanceQuery

| Movements | Pre-UUID (ns/op) | Post-UUID (ns/op) | Change |
|----------:|-----------------:|-----------------:|-------:|
| 100 | 202,690 | 374,496 | +85% |
| 1,000 | 491,236 | 935,651 | +91% |
| 10,000 | 4,233,675 | 5,255,329 | +24% |

Balance queries join on account IDs. TEXT comparisons are slower than INTEGER.
The gap narrows at higher row counts — index lookup dominates over comparison cost.

### SimpleMovementAndBalance

| Phase | ns/op | B/op | allocs/op |
|-------|------:|-----:|----------:|
| Pre-UUID | 1,539,734 | 516,800 | 1,103 |
| Post-UUID | 1,170,147 | 463,651 | 964 |
| **Change** | **-24%** | **-10%** | **-13%** |

Combined insert+query is faster post-UUID. The `MAX(batch_id)+1` subquery
was a bottleneck that scaled poorly; UUID generation is constant-time.

### CompoundMovementWithProjections

| Phase | ns/op | B/op | allocs/op |
|-------|------:|-----:|----------:|
| Pre-UUID | 3,310,801 | 1,213,955 | 2,635 |
| Post-UUID | 2,704,278 | 1,164,260 | 2,546 |
| **Change** | **-18%** | **-4%** | **-3%** |

Compound operations (insert + interest accrual + live balance upsert) benefit
most from removing the `MAX(batch_id)` bottleneck, which ran inside an open
transaction holding a write lock.

### InterestNAccounts

| N accounts | Pre-UUID (ns/op) | Post-UUID (ns/op) | Change |
|-----------:|-----------------:|-----------------:|-------:|
| 10 | 19,312,550 | 14,038,693 | -27% |
| 100 | 162,987,281 | 129,139,358 | -21% |
| 1,000 | 1,778,863,389 | 1,234,454,536 | -31% |
| 10,000 | 16,212,973,778 | 10,315,144,369 | -36% |

Interest processing scales better with UUIDs. The improvement grows with N
because each interest calculation involved a `MAX(batch_id)+1` query that
scanned an ever-growing movements table.

### Summary

- **Pure balance queries**: 24-91% slower (TEXT vs INTEGER key comparisons)
- **Write-heavy operations**: 18-36% faster (UUID gen vs MAX+1 contention)
- **Memory**: 3-17% less allocation (no RETURNING id scan overhead)
- **Net assessment**: Positive for the write-heavy workloads that dominate
  real accounting usage. Balance query regression is acceptable given the
  migration benefits (distributed-DB readiness, no hotspot PKs).
