# Changelog

## [Unreleased]

## [0.2.19] - 2026-03-16

 - Forcing UTC time

## [0.2.18] - 2026-03-16

 - fix using rfc3339 format timestamp

## [0.2.17] - 2026-03-16

 - refactor to goluca syntax

### WTrefactor — DB layer catch-up (2026-03-16)

#### Phase 1: UUID Primary Keys + Account Path Utilities
- All PKs migrated from `SERIAL`/`int64` to UUID/`string` (accounts, movements, balances_live)
- All FK references changed from `INTEGER` to `TEXT`
- Batch IDs now UUID (`uuid.New().String()`), removed `nextBatchID` MAX+1 pattern
- `google/uuid` promoted to direct dependency
- Added `BuildFullPath()` and `Account.RebuildFullPath()` path utilities
- MemLedger changed from array-indexed to `map[string]*Account`

#### Phase 2: Knowledge DateTime + Bitemporal Queries
- `MovementInput.KnowledgeTime` field for explicit knowledge time passthrough
- Import preserves `%datetime` knowledge timestamps from goluca files
- Export emits `%datetime` when knowledge_time differs from value_time
- `BalanceAsOf(accountID, valueTime, knowledgeTime)` bitemporal balance query
- `FirstMovementTime`/`LastMovementTime` time-range discovery helpers

#### Phase 3: Directive & Metadata Tables + Time Series
- 8 new tables: options, commodities, commodity_metadata, aliases, customers, customer_metadata, data_points, movement_metadata
- `Account.OpenedAt` field + `opened_at` column for open directives
- Full CRUD: UpsertOption, CreateCommodity, CreateAlias, ResolveAlias, CreateCustomer, SetMovementMetadata, etc.
- `DataPointValue` typed values (string/number/boolean/null) with `InferDataPointType`
- Time series: SetDataPoint, GetDataPoint, GetDataPointAsOf, DataPointRange
- Import stores all directives, metadata, typed data points; resolves aliases
- Export queries all directive tables and populates GolucaFile

#### Phase 4: Period Anchor in DB
- `period_anchor VARCHAR(1)` column on movements table
- `Movement.PeriodAnchor` and `MovementInput.PeriodAnchor` fields
- Import/export preserve period anchors (`^`, `$`) through DB round-trip

#### Phase 5: Transaction Operations + Search
- `AddMovementToBatch` appends movements to existing batches
- `DiffTransactions` compares two Transaction structs (movements, metadata, datetime, payee)
- `Events` merges movements and data points into a unified timeline
- `SearchMovements`/`CountMovements` with dynamic WHERE clause (account, path prefix, time range, description, code, amount range, batch, pagination)

#### Phase 6: API Endpoints
- 15 new HTTP endpoints: balance-as-of, first/last-time, add-to-batch, search, events, options CRUD, commodities/aliases/customers listing, data-points CRUD, import/export
- `NewServer` now accepts `*SQLLedger` for full feature access

#### Phase 7: Round-Trip Test Documentation
- `roundTrip()` test helper for import → DB → export → re-parse cycle
- 8 new comprehensive round-trip tests covering pending, zero amounts, multiple transactions, mixed directives, DiffTransactions verification, data point types, metadata

#### Phase 8: Benchmark Comparison
- Pre/post UUID benchmark comparison saved in `benchmarks/uuid-migration/`
- Write-heavy operations 18-36% faster (UUID gen vs MAX+1 contention)
- Pure balance queries 24-91% slower (TEXT vs INTEGER keys)
- Memory allocations 3-17% lower (no RETURNING id scan overhead)

## [0.2.16] - 2026-03-14

 - Changing to number

## [0.2.15] - 2026-03-13

 - Adding AER research

## [0.2.14] - 2026-03-13

 - Just updating docs

## [0.2.13] - 2026-03-12

 - Just refreshe

## [0.2.12] - 2026-03-12

 - Adding numbers

## [0.2.11] - 2026-03-12

 - Merging in decimal research

## [0.2.10] - 2026-03-07

 - Updating versions

## [0.2.9] - 2026-03-07

 - Adding numbers section

## [0.2.8] - 2026-03-06

 - add ing DB tables docuemntation

## [0.2.7] - 2026-03-05

 - docmentation release

## [0.2.6] - 2026-03-04

 - Fleshing out API interface and creating ledger documentation

## [0.2.5] - 2026-03-04

 - Restructuring and adding documenation

## [0.2.4] - 2026-03-04

 - Changing to local gotreesitter

## [0.2.3] - 2026-03-03

 - Improving documentation

## [0.2.2] - 2026-03-03

 - using task-plus md2svg for change

## [0.2.1] - 2026-03-03

 - Starting to convert to and from plain text files
