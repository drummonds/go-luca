## Purpose

Compare go-luca performance before and after the UUID primary key migration
(Phase 1 of the WTrefactor plan).

### What changed

- All primary keys: `SERIAL` (auto-increment integer) → `TEXT` (UUID v4)
- All foreign key references: `INTEGER` → `TEXT`
- Batch IDs: sequential `MAX(batch_id) + 1` → `uuid.New().String()`
- Movement IDs: `RETURNING id` → pre-generated UUID

### What to look for

- **RecordMovement throughput**: UUID generation replaces MAX+1 query —
  should be similar or better at scale (no contention on MAX).
- **Balance queries**: TEXT comparisons vs INTEGER comparisons in JOINs
  and WHERE clauses. May show slight regression due to longer keys.
- **Memory**: UUID strings (36 bytes) vs int64 (8 bytes) — expect
  higher allocations per operation.
- **Interest processing**: Aggregate effect across N accounts.
