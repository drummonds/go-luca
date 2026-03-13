# accounts

## Description

Chart of accounts. Each account has a hierarchical path (Type:Product:AccountID:Address) and belongs to one of five fundamental types: Asset, Liability, Equity, Income, Expense. Amounts are stored as integers at the precision defined by exponent (e.g. -2 for pence).  


<details>
<summary><strong>Table Definition</strong></summary>

```sql
CREATE TABLE accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    full_path TEXT NOT NULL UNIQUE,
    account_type TEXT NOT NULL,
    product TEXT NOT NULL DEFAULT '',
    account_id TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    is_pending INTEGER DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'GBP',
    exponent INTEGER NOT NULL DEFAULT -2,
    annual_interest_rate TEXT NOT NULL DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
)
```

</details>

## Columns

| Name                 | Type    | Default         | Nullable | Children                                                    | Parents | Comment                                                                        |
| -------------------- | ------- | --------------- | -------- | ----------------------------------------------------------- | ------- | ------------------------------------------------------------------------------ |
| account_id           | TEXT    | ''              | false    |                                                             |         | Specific account identifier within the product                                 |
| account_type         | TEXT    |                 | false    |                                                             |         | One of: Asset, Liability, Equity, Income, Expense                              |
| address              | TEXT    | ''              | false    |                                                             |         | Sub-address within the account (e.g. branch). 'Pending' marks pending accounts |
| annual_interest_rate | TEXT    | 0               | false    |                                                             |         | Annual interest rate as a decimal (0.045 = 4.5%)                               |
| created_at           | TEXT    | datetime('now') | true     |                                                             |         | Timestamp when the account was created                                         |
| currency             | TEXT    | 'GBP'           | false    |                                                             |         | ISO 4217 currency code (e.g. GBP, USD)                                         |
| exponent             | INTEGER | -2              | false    |                                                             |         | Decimal exponent for amount precision (-2 = pence, -5 = high precision)        |
| full_path            | TEXT    |                 | false    |                                                             |         | Hierarchical account path, e.g. Asset:Bank:Current:Main                        |
| id                   | INTEGER |                 | true     | [balances_live](balances_live.md) [movements](movements.md) |         | Auto-incrementing primary key                                                  |
| is_pending           | INTEGER | 0               | true     |                                                             |         | True if this is a pending/suspense account                                     |
| product              | TEXT    | ''              | false    |                                                             |         | Product category within the account type                                       |

## Constraints

| Name                        | Type        | Definition         |
| --------------------------- | ----------- | ------------------ |
| id                          | PRIMARY KEY | PRIMARY KEY (id)   |
| sqlite_autoindex_accounts_1 | UNIQUE      | UNIQUE (full_path) |

## Indexes

| Name                        | Definition         |
| --------------------------- | ------------------ |
| sqlite_autoindex_accounts_1 | UNIQUE (full_path) |

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
  INTEGER id "Auto-incrementing primary key"
  INTEGER is_pending "True if this is a pending/suspense account"
  TEXT product "Product category within the account type"
}
"balances_live" {
  INTEGER account_id "Account this balance belongs to (references accounts.id)"
  INTEGER balance "End-of-day balance in smallest currency unit"
  TEXT balance_date "Date of the balance snapshot (start of day)"
  INTEGER id "Auto-incrementing primary key"
  TEXT updated_at "When this balance was last recomputed"
}
"movements" {
  INTEGER amount "Transfer amount in smallest currency unit (integer at account exponent)"
  INTEGER batch_id "Groups related movements into a single atomic transaction"
  INTEGER code "Movement category: 0=normal, 1=interest accrual (TigerBeetle-inspired)"
  TEXT description "Human-readable description of the movement"
  INTEGER from_account_id "Source account (references accounts.id)"
  INTEGER id "Auto-incrementing primary key"
  TEXT knowledge_time "When the system recorded this movement (knowledge date)"
  INTEGER ledger "Partition identifier for multi-ledger setups (TigerBeetle-inspired)"
  INTEGER pending_id "Two-phase commit: references pending movement to post/void (0=N/A)"
  INTEGER to_account_id "Destination account (references accounts.id)"
  INTEGER user_data_64 "Arbitrary external reference for application use"
  TEXT value_time "When the movement economically occurred (value date)"
}
```

---

> Generated by [tbls](https://github.com/k1LoW/tbls)
