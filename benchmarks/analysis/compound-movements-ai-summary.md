Compound movement (write-time projection of interest + live balance) vs simple
movement + balance query, benchmarked on real PostgreSQL.

Results placeholder — update after first run with actual TPS numbers.

The trade-off: eliminating end-of-day batch processing in exchange for extra per-write
overhead. The compound path does 7 SQL operations in a single transaction (insert
movement, compute balance, delete old accrual, insert new accrual, delete old live
balance, recompute balance, insert live balance).
