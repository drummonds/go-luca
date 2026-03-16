# go-luca

## Description

Movement-based double-entry bookkeeping database schema

## Tables

| Name                                        | Columns | Comment                                                                                                                                                                                                                                                                          | Type  |
| ------------------------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- |
| [accounts](accounts.md)                     | 12      | Chart of accounts. Each account has a hierarchical path (Type:Product:AccountID:Address) and belongs to one of five fundamental types: Asset, Liability, Equity, Income, Expense. Amounts are stored as integers at the precision defined by exponent (e.g. -2 for pence).<br /> | table |
| [aliases](aliases.md)                       | 4       |                                                                                                                                                                                                                                                                                  | table |
| [balances_live](balances_live.md)           | 5       | Pre-computed end-of-day balance snapshots. Updated transactionally when movements are recorded via RecordMovementWithProjections. Avoids expensive SUM queries for frequently accessed balances.<br />                                                                           | table |
| [commodities](commodities.md)               | 4       |                                                                                                                                                                                                                                                                                  | table |
| [commodity_metadata](commodity_metadata.md) | 4       |                                                                                                                                                                                                                                                                                  | table |
| [customer_metadata](customer_metadata.md)   | 4       |                                                                                                                                                                                                                                                                                  | table |
| [customers](customers.md)                   | 6       |                                                                                                                                                                                                                                                                                  | table |
| [data_points](data_points.md)               | 7       |                                                                                                                                                                                                                                                                                  | table |
| [movement_metadata](movement_metadata.md)   | 4       |                                                                                                                                                                                                                                                                                  | table |
| [movements](movements.md)                   | 13      | Core transaction records. Each movement transfers an integer amount from one account to another. Movements with the same batch_id form a linked transaction (compound entry). Inspired by TigerBeetle's transfer model with code, ledger, and pending_id fields.<br />           | table |
| [options](options.md)                       | 4       |                                                                                                                                                                                                                                                                                  | table |

## Relations

```mermaid
erDiagram

"balances_live" }o--|| "accounts" : "balances_live.account_id -> accounts.id"
"movements" }o--|| "accounts" : "movements.from_account_id -> accounts.id"
"movements" }o--|| "accounts" : "movements.to_account_id -> accounts.id"

"accounts" {
  TEXT account_id "Specific account identifier within the product"
  TEXT account_type "One of: Asset, Liability, Equity, Income, Expense"
  TEXT address "Sub-address within the account (e.g. branch). 'Pending' marks pending accounts"
  TEXT annual_interest_rate "Annual interest rate as a decimal (0.045 = 4.5%)"
  TEXT created_at "Timestamp when the account was created"
  TEXT currency "ISO 4217 currency code (e.g. GBP, USD)"
  INTEGER exponent "Decimal exponent for amount precision (-2 = pence, -5 = high precision)"
  TEXT full_path "Hierarchical account path, e.g. Asset:Bank:Current:Main"
  TEXT id PK "Auto-incrementing primary key"
  INTEGER is_pending "True if this is a pending/suspense account"
  TEXT opened_at ""
  TEXT product "Product category within the account type"
}
"aliases" {
  TEXT account_path ""
  TEXT created_at ""
  TEXT id PK ""
  TEXT name ""
}
"balances_live" {
  TEXT account_id "Account this balance belongs to (references accounts.id)"
  INTEGER balance "End-of-day balance in smallest currency unit"
  TEXT balance_date "Date of the balance snapshot (start of day)"
  TEXT id PK "Auto-incrementing primary key"
  TEXT updated_at "When this balance was last recomputed"
}
"commodities" {
  TEXT code ""
  TEXT created_at ""
  TEXT datetime ""
  TEXT id PK ""
}
"commodity_metadata" {
  TEXT commodity_id ""
  TEXT id PK ""
  TEXT key ""
  TEXT value ""
}
"customer_metadata" {
  TEXT customer_id ""
  TEXT id PK ""
  TEXT key ""
  TEXT value ""
}
"customers" {
  TEXT account_path ""
  TEXT created_at ""
  TEXT id PK ""
  TEXT max_balance_amount ""
  TEXT max_balance_commodity ""
  TEXT name ""
}
"data_points" {
  TEXT created_at ""
  TEXT id PK ""
  TEXT knowledge_time ""
  TEXT param_name ""
  TEXT param_type ""
  TEXT param_value ""
  TEXT value_time ""
}
"movement_metadata" {
  TEXT batch_id ""
  TEXT id PK ""
  TEXT key ""
  TEXT value ""
}
"movements" {
  INTEGER amount "Transfer amount in smallest currency unit (integer at account exponent)"
  TEXT batch_id "Groups related movements into a single atomic transaction"
  INTEGER code "Movement category: 0=normal, 1=interest accrual (TigerBeetle-inspired)"
  TEXT description "Human-readable description of the movement"
  TEXT from_account_id "Source account (references accounts.id)"
  TEXT id PK "Auto-incrementing primary key"
  TEXT knowledge_time "When the system recorded this movement (knowledge date)"
  INTEGER ledger "Partition identifier for multi-ledger setups (TigerBeetle-inspired)"
  INTEGER pending_id "Two-phase commit: references pending movement to post/void (0=N/A)"
  TEXT period_anchor ""
  TEXT to_account_id "Destination account (references accounts.id)"
  INTEGER user_data_64 "Arbitrary external reference for application use"
  TEXT value_time "When the movement economically occurred (value date)"
}
"options" {
  TEXT created_at ""
  TEXT id PK ""
  TEXT key ""
  TEXT value ""
}
```

---

> Generated by [tbls](https://github.com/k1LoW/tbls)
