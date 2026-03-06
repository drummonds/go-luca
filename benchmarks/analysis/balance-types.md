# Balance Types Benchmark

## Purpose

This is a simulation of realtime balance lookup without caching. In a bank the most
time critical element is movement in of money and then movement out of money. This is
where this query starts to be realistic.

This started out testing summing  three PostgreSQL data types (BIGINT, DOUBLE PRECISION, NUMERIC) for calculated balances or sums.  However it is clear that sum is about 40-50% slower for NUMERIC and the other two are about the same speed.

This is a test of how much it slows down a calculation if you have live balances with only a single row or you have to query with a date.  The idea is that there will be a balance with todays date and balance with tomorrows date.  As the query rolls over midnight you will return different data due to future dated postings.   I want a realistic test with up to a million accounts in the table.

## Analysis

This shows that future date timing has marginal effect on accessing balance information
for millions of accounts so it  is practical from this point of view.

With the (account_id, value_time DESC) index, all three types deliver sub-millisecond
point-in-time lookups even at 36.5M rows. The data type choice has negligible impact
on indexed single-row retrieval.

Use BIGINT (integer cents/pence) for balance storage. It matches go-luca's existing
int64 amount model, avoids floating-point rounding (DOUBLE), and has no NUMERIC
overhead. Reserve NUMERIC for external reporting views where decimal display formatting
is needed.

## Summary

All three data types (BIGINT, DOUBLE PRECISION, NUMERIC) perform equivalently for
indexed point-in-time balance lookups across all tested scales (100K to 36.5M rows).
P50 latencies remain 100-220us regardless of type or table size, confirming that the
B-tree index dominates query cost — the arithmetic type is irrelevant for single-row
retrieval.

BIGINT is recommended: zero precision loss, smallest storage footprint, and native
alignment with go-luca's int64 amount representation.
