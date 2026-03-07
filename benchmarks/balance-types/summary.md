All three data types (BIGINT, DOUBLE PRECISION, NUMERIC) perform equivalently for
indexed point-in-time balance lookups across all tested scales (100K to 36.5M rows).
P50 latencies remain 100-220us regardless of type or table size, confirming that the
B-tree index dominates query cost — the arithmetic type is irrelevant for single-row
retrieval.

BIGINT is recommended: zero precision loss, smallest storage footprint, and native
alignment with go-luca's int64 amount representation.
