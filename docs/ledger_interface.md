# Ledger Interface

The `Ledger` interface decouples accounting operations from storage backends.

## Method Groups

### Accounts
- `CreateAccount` — insert a new account by full path
- `GetAccount` — look up by full path
- `GetAccountByID` — look up by numeric ID
- `ListAccounts` — list all, optionally filtered by type

### Movements
- `RecordMovement` — single movement between two accounts (same exponent enforced)
- `RecordLinkedMovements` — batch of movements sharing a batch ID
- `RecordMovementWithProjections` — movement with interest accrual and live balance upsert

### Balances
- `Balance` — current all-time balance
- `BalanceAt` — balance at a point in time (value_time <= t)
- `BalanceByPath` — aggregate balance across accounts matching a path prefix
- `DailyBalances` — day-by-day closing balances over a date range

### Positions
Stored end-of-day projections: an account's balance and its accrued-but-unapplied
interest (an exact `Fraction`) per day. `RecordMovementWithProjections` rewrites
both accounts' positions from the movement's day onwards; a backdated movement
corrects every later day. Published as the contract views
`contract_ledger_eod_positions` (one row per account per day; cheap) and
`contract_ledger_live_positions` (the position on the business day, else the
previous one, plus movements since), with money as NUMERIC in major units.

- `Project` — write a day's position with no movement: the ledger's balance for that day and the caller's accrued interest
- `PositionAt` — the latest position on or before a day
- `AdvanceDay` — move the business day on before the pass projects it (`SQLLedger` only); the live view reads the day's and the previous business day's positions as two slices of the day index
- `BusinessDay` — the business day and the one it advanced from

### Interest
- `EnsureInterestAccounts` — create system accounts for interest processing
- `CalculateDailyInterest` — one day's interest for one account
- `RunDailyInterest` — process interest for all interest-bearing accounts
- `RunInterestForPeriod` — run daily interest over a date range

### Import/Export
Import and export operate on journals — the plain text record of accounting movements. The .goluca format is one of several [plain text accounting formats](https://github.com/drummonds/plain-text-accounting-formats).

- `ListMovements` — all movements with resolved account paths
- `Export` — write the journal as .goluca text
- `Import` / `ImportString` — read a .goluca journal and record its movements

## Backend Capabilities

| Method Group | SQLLedger | MemLedger |
|---|---|---|
| Accounts | Full | Full |
| Movements (basic) | Full | Full |
| RecordMovementWithProjections | Full | Stub |
| Balances (basic) | Full | Full |
| BalanceByPath | Full | Stub |
| Positions (Project, PositionAt) | Full | Stub |
| Interest | Full | Stub |
| Import/Export | Full | Stub |

Stubbed methods return `ErrNotImplemented`.

## Bitemporal Notes

Movements carry two timestamps:
- **value_time** — when the economic event occurred
- **knowledge_time** — when the system recorded it (defaults to NOW)

`BalanceAt` queries against value_time. Future extensions could support knowledge-time queries (as-of / as-known-at) for full bitemporal reporting.

## Future Directions

- **Compact MemLedger** — columnar storage for read-heavy analytics
- **Large MemLedger** — sharded maps for high-concurrency writes
- **pgx backend** — high-performance PostgreSQL via pgx/v5 (bypassing database/sql)
- **Bitemporal queries** — knowledge-time-aware balance methods
