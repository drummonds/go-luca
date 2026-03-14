# go-luca Project Instructions

## Protected files
Files matching `benchmarks/*/purpose.md` are user-authored.
Never modify these files unless explicitly asked in the conversation.

## 128-bit Amount Migration Plan

The project currently uses bare `int64` for all amounts. The eventual goal is 128-bit
integers. This section documents how to make that migration as cheap as possible.

### Current state
- `type Amount int64` is defined in `luca.go` and used across all amount-carrying fields
- `Movement.Amount`, `MovementInput.Amount`, `LiveBalance.Balance`, `DailyBalance.Balance`,
  `InterestResult.OpeningBalance/InterestAmount/ClosingBalance` all use `Amount`
- `Ledger` interface methods use `Amount` for amount params/returns
- `decimal.go` helpers (`IntToDecimal`, `DecimalToInt`, `ScaleAmount`) use `Amount`
- DB schema uses `BIGINT` — `database/sql.Scan` handles `Amount` via reflection
- Account IDs, batch IDs, movement IDs remain `int64`

### Rules to follow (keep migration cost low)
1. **Always use `Amount` for new amount fields** — never bare `int64` for monetary values.
2. **Keep amount arithmetic in `decimal.go`** — don't scatter raw `+`, `-`, `*` on amounts
   across files. Centralised helpers become a single migration point.
3. **No amount logic in SQL** — keep `SUM(amount)` as the only SQL-side arithmetic. Avoid
   SQL expressions like `amount * rate` that would need type-aware rewriting.
4. **Don't widen the Ledger interface unnecessarily** — every new method that takes/returns
   `Amount` is another signature to change later.

### Migration checklist (int64 → 128-bit)
1. **Change `Amount` backing type** — update `type Amount int64` to wrap whatever 128-bit
   representation is chosen (`big.Int`, `uint128` struct, or stdlib if available by then).
   Add `Amount.Add()`, `Amount.Sub()`, `Amount.Cmp()` methods if the backing type doesn't
   support Go arithmetic operators.
2. **Update `decimal.go`** — adapt `IntToDecimal`, `DecimalToInt`, `ScaleAmount` internals
   (the signatures already use `Amount`).
3. **Fix arithmetic** — if the 128-bit type doesn't support `+`, `-`, `*` operators,
   replace inline arithmetic with method calls. The compiler will find every site.
4. **DB migration** — Postgres: `ALTER TABLE movements ALTER COLUMN amount TYPE NUMERIC`.
   pglike/SQLite: recreate tables (no ALTER COLUMN TYPE support), migrate data.
   `SUM(amount)` works unchanged on `NUMERIC`.
5. **Update scan/serialisation** — implement `sql.Scanner`/`driver.Valuer` on `Amount` for
   DB round-tripping. JSON: implement `MarshalJSON`/`UnmarshalJSON` (string representation
   to avoid JS number precision loss).
6. **Account IDs stay `int64`** — only amounts need 128-bit; IDs and batch IDs are fine as-is.
