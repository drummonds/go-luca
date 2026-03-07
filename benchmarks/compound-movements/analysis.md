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
