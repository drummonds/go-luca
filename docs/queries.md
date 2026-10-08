# Queries

Every query go-luca runs or publishes, the shape chosen for it, the
shapes chosen against, and how each fits the budget of the screen or
process that reads it. The figures and budgets are shared with the
components that read the contract views (gobank's book, pass and
statements), so a change on either side is measured against one table.

Statement: a query's cost is written in the ledger's scale terms, not
seconds, so it stays true as the data grows. The measured figures that
give it seconds are in the last section, dated.

## Scale terms

| Term | Meaning | gobank prod 2026-10-07 (cx23) |
|---|---|---|
| A | accounts | 84k customer accounts |
| M | movements | about 2 per account per day when active |
| D | days projected | 404 |
| P | position rows, A × D | 34M |
| m_a | one account's movements, all time | tens to hundreds |
| s_a | one account's movements since its latest position | 0 to a few |
| probe | one index lookup, log of the table | microseconds on PG, less on pglike |

Shapes, cheapest first: **point** (a probe), **range** (a probe then a
scan of s_a or m_a rows), **set** (one scan of a whole table or
grouping of it), **correlated** (a probe or range per account, A times).

## Writes

| Query | Shape | Cost | Chosen | Not chosen |
|---|---|---|---|---|
| `RecordMovement` | insert | 1 | one insert, knowledge_time written by the ledger | DB default for knowledge_time (#6: whole seconds on pglike) |
| `RecordLinkedMovements` | n inserts, one tx | n | shared batch id | one tx per movement |
| `RecordMovementWithProjections` | insert + 2 position moves | 1 + 2 × (probe + s_a) | positions on and after the day move by the amount (one UPDATE); only a day with no row yet is built from the one before plus a range sum (v0.3.1) | rebuild every later day from the previous one (v0.3.0: cost grew with days after and with a hot account's movements); interest posted at write time (v0.2.x compound, removed: the product engine owns interest) |
| `Project(account, day, accrued)` | probe + range + upsert | probe + s_a | previous row, movements between, write the day's balance and accrual | summing the account's history (m_a) |
| backdated movement | per later day: range | days after × s | `reproject` rebuilds from the movement's day forward, so every later day is right | refusing backdated value times |

## Reads of one account

| Query | Shape | Cost | Chosen | Not chosen | Open |
|---|---|---|---|---|---|
| `Balance(account)` | 2 range sums | m_a | two index range sums (to, from) over all time | | position + movements since (probe + s_a); the pass calls `Balance` per account per day, so this is the pass's cost floor |
| `BalanceAt(account, t)` | 2 range sums | m_a to t | as `Balance` with `value_time <= t` | | nearest position before t + delta |
| `BalanceAsOf(account, value, knowledge)` | 2 range sums + filter | m_a | the value_time index, knowledge_time filtered in the scan | index on knowledge_time (no consumer yet) | |
| `DailyBalances(account, from, to)` | days × `BalanceAt` | days × m_a | convenience over `BalanceAt`, predates positions | | read `contract_ledger_eod_positions` (days × probe) |
| `PositionAt(account, day)` | point | probe | index backwards on (account, balance_date) | | |
| `First/LastMovementTime(account)` | MIN/MAX over from OR to | m_a | two index ranges, OR-ed | | |
| `SearchMovements` / `CountMovements` | filtered scan | rows matched | account filter is the OR of two indexed columns; path prefix joins accounts with LIKE | | |
| `contract_ledger_movements WHERE account` | range + 2 joins | m_a | the movements table with both paths joined, filter pushed down on both drivers | | |
| `contract_ledger_live_positions WHERE account` | see live view | PG: probe + s_a; pglike: **the whole view** | the one-account read gobank's product engine makes | | pglike cannot push the filter into grouped subqueries: the v0.3.1 correlated form is 0.6 ms, the #7 grouped rewrite 90 ms (1k accounts). Any grouped rewrite needs a one-account path of its own on pglike |

## Reads of the whole ledger

| Query | Shape | Cost | Chosen | Not chosen | Open |
|---|---|---|---|---|---|
| `Positions(day)` | set | P | grouped `MAX(balance_date)` joined back (one scan of balances_live) | `DISTINCT ON`, `LATERAL` (Postgres only; views and queries must run on pglike); correlated MAX per account | a `(balance_date, account_id)` index would bound the scan to days ≤ day (#9) |
| `BalanceByPath(prefix, at)` | set | M matching | movements joined to accounts on an OR, LIKE on full_path | an account hierarchy table | reporting only; no screen reads it |
| `contract_ledger_eod_positions` | pass-through | rows selected | positions with paths and money in major units; the cheap view | | |
| `contract_ledger_latest_positions` | set | P | grouped MAX, once, so consumers stop bolting their own correlated `MAX(day)` onto the eod view (#7, first part) | | |
| `contract_ledger_live_positions` | correlated (v0.3.1) | A × (probe + 2 × (probe + s_a)) | latest row per account by correlated MAX, plus two correlated sums | | **does not meet the dashboard budget at prod scale** (12 to 31 s). Candidates below |
| `contract_ledger_live_positions` (#7 grouped rewrite, measured and dropped) | set | 3P + M | | three evaluations of the latest-row set, one per LEFT JOIN: slower than v0.3.1 at 1k and 10k accounts on both drivers | |

### Live positions: the candidates

The live view is the one query whose chosen shape is wrong at scale. The
budget is the dashboard's: every customer account summed within a poll
(1 s, behind a 2 s book cache) at 100k accounts and 400 days, P = 40M.

| Candidate | Cost | Meets budget at P = 40M | Cost elsewhere | Status |
|---|---|---|---|---|
| correlated MAX + 2 sums (v0.3.1) | A × 3 probes | no: 12 to 31 s measured at A = 84k | none | shipped |
| grouped MAX, three times (#7 as written) | 3P + M | no: scans P three times | none | measured, rejected |
| grouped MAX once, materialised CTE | P + M | no: one scan of P is seconds at 40M | none | measured (289 ms at P = 918k vs 773 ms), rejected for scale |
| business-day row: today's else yesterday's row via `(balance_date, account_id)` index | A × 2 probes | likely: no scan of P; A probes | a `ledger_day` row the pass advances; the index (#9); a stale account (neither day) falls back to a probe | spike in `cmd/bench-compound-movements` |
| latest row kept on write: a `balances_latest` table or `is_latest` flag maintained by `Project` | A | yes | one extra write per projection; a backdated reproject must repair it | not built |
| consumer cache: gobank's book refreshed in the background, requests never wait | 0 per request | yes for the screen; the read still runs once per refresh | staleness of one refresh; gobank story | planned on the gobank side |

Recommendation: the business-day row, because the pass already defines
the day and the index is wanted for `Positions(day)` and the pass's
`unprojected` anyway (#9); the latest-row table is the fallback if the
stale-account probe shows up in the figures. The grouped forms stay for
`contract_ledger_latest_positions`, which is read once a day, not once a
poll.

## Consumers and budgets

What each reader does with the queries above, and the time it has.

| Reader | Queries | Cadence | Budget | Shape it needs |
|---|---|---|---|---|
| gobank dashboard book (savings, lending per family) | `contract_ledger_live_positions` summed over every customer account | 1 s poll, 2 s cache | under 1 s | set or A probes |
| gobank P&L accrued interest | eod view + correlated `MAX(day)` per account (`accruedByFamily`) | per page | under 1 s | `contract_ledger_latest_positions` |
| gobank product balances | sum of every movement per product through `contract_ledger_movements` | per page | under 1 s | M scan today; latest positions once the view exists |
| gobank start-of-day pass | `unprojected` (NOT EXISTS on eod view per account, paged), then per account `PositionAt`, `Balance`, `Project` | once a day, every account | the account-days/12h figure | per account: probes + s_a. `Balance` is m_a; see Open |
| gobank product engine, one account | `contract_ledger_live_positions WHERE account_id` | per request | ms | point on both drivers |
| gobank statements | `contract_ledger_movements WHERE account` | per request | ms | range |
| gobank bank reserves | `PositionAt`, `Balance`, `Project` on one account | per day | ms | point |
| go-luca API (`/balances`) | `Balance`, `BalanceAt`, `DailyBalances` | per request | ms | range |

## Measured figures

Whole-ledger live read of the position views, this repo's
`BenchmarkPositionViewsScale` (90 days of positions per account, a
quarter of accounts mid-pass), 2026-10-08, laptop, PG 16 in podman:

| Query | A = 1k PG | A = 10k PG | A = 1k pglike |
|---|---|---|---|
| live view, v0.3.1 correlated | 9 ms | 504 ms | 16 ms |
| live view, #7 grouped (dropped) | 51 ms | 1254 ms | 100 ms |
| latest view, correlated MAX per account (what gobank bolts on) | 288 ms | 2922 ms | 113 ms |
| latest view, grouped (shipped) | 17 ms | 292 ms | 35 ms |
| live view one account, v0.3.1 | 0.24 ms | 0.40 ms | 0.57 ms |
| live view one account, #7 grouped | 0.40 ms | 0.44 ms | 90 ms |

Production, gobank v0.18 on cx23 (4 GB shared), A = 84k, D = 404,
2026-10-07: dashboard 12 to 31 s when the book cache misses, 0.06 s
when it hits; P&L and Runtime pages over 90 s.

Pass rate, gobank perf workflow (`benchmark.md` there): 18.4M
account-days per 12 h on cx23, 40.9M on ccx33, at v0.12 with 120k
accounts.

Write path, in-process pglike (`task benchmark`): `RecordMovement`
about 2,300/s; `RecordMovementWithProjections` constant per movement on
a hot account (`BenchmarkHotAccountProjection`).

## Keeping this true

A query is added here when it is added to the code, with its shape and
cost in scale terms. A figure is added when it is measured, with the
date and scale. A candidate moves to Chosen or Not chosen with the
measurement that decided it, so the reason survives the code.
