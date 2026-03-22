# accounts

## Description

Chart of accounts. Each account has a hierarchical path (Type:Product:AccountID:Address) and belongs to one of five fundamental types: Asset, Liability, Equity, Income, Expense. An account optionally belongs to a customer (many accounts per customer). Amounts are stored as integers at the precision defined by the commodity's exponent.  


<details>
<summary><strong>Table Definition</strong></summary>

```sql
CREATE TABLE accounts (
    id TEXT PRIMARY KEY,
    full_path TEXT NOT NULL UNIQUE,
    account_type TEXT NOT NULL,
    product TEXT NOT NULL DEFAULT '',
    account_id TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    is_pending INTEGER DEFAULT 0,
    commodity TEXT NOT NULL DEFAULT 'GBP' REFERENCES commodities(code),
    customer_id TEXT REFERENCES customers(id),
    gross_interest_rate TEXT NOT NULL DEFAULT 0,
    interest_method TEXT NOT NULL DEFAULT '',
    interest_accumulator INTEGER NOT NULL DEFAULT 0,
    opened_at TEXT,
    created_at TEXT DEFAULT (datetime('now'))
)
```

</details>

## Columns

| Name                 | Type    | Default         | Nullable | Children                                                    | Parents                       | Comment                                                                          |
| -------------------- | ------- | --------------- | -------- | ----------------------------------------------------------- | ----------------------------- | -------------------------------------------------------------------------------- |
| account_id           | TEXT    | ''              | false    |                                                             |                               | Specific account identifier within the product                                   |
| account_type         | TEXT    |                 | false    |                                                             |                               | One of: Asset, Liability, Equity, Income, Expense                                |
| address              | TEXT    | ''              | false    |                                                             |                               | Sub-address within the account (e.g. branch). 'Pending' marks pending accounts   |
| commodity            | TEXT    | 'GBP'           | false    |                                                             | [commodities](commodities.md) | Commodity code (FK to commodities.code)                                          |
| created_at           | TEXT    | datetime('now') | true     |                                                             |                               | Timestamp when the account was created                                           |
| customer_id          | TEXT    |                 | true     |                                                             | [customers](customers.md)     | Optional owning customer (FK to customers.id). A customer may have many accounts |
| full_path            | TEXT    |                 | false    | [aliases](aliases.md)                                       |                               | Hierarchical account path, e.g. Asset:Bank:Current:Main                          |
| gross_interest_rate  | TEXT    | 0               | false    |                                                             |                               | Gross annual interest rate as a decimal (0.045 = 4.5%)                           |
| id                   | TEXT    |                 | true     | [balances_live](balances_live.md) [movements](movements.md) |                               | UUID primary key                                                                 |
| interest_accumulator | INTEGER | 0               | false    |                                                             |                               | Sub-unit fractions at extended precision (method-dependent)                      |
| interest_method      | TEXT    | ''              | false    |                                                             |                               | Interest calculation method (e.g. simple_daily)                                  |
| is_pending           | INTEGER | 0               | true     |                                                             |                               | True if this is a pending/suspense account                                       |
| opened_at            | TEXT    |                 | true     |                                                             |                               | When the account was opened                                                      |
| product              | TEXT    | ''              | false    |                                                             |                               | Product category within the account type                                         |

## Constraints

| Name                        | Type        | Definition                                                                                               |
| --------------------------- | ----------- | -------------------------------------------------------------------------------------------------------- |
| - (Foreign key ID: 0)       | FOREIGN KEY | FOREIGN KEY (customer_id) REFERENCES customers (id) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE   |
| - (Foreign key ID: 1)       | FOREIGN KEY | FOREIGN KEY (commodity) REFERENCES commodities (code) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE |
| id                          | PRIMARY KEY | PRIMARY KEY (id)                                                                                         |
| sqlite_autoindex_accounts_1 | PRIMARY KEY | PRIMARY KEY (id)                                                                                         |
| sqlite_autoindex_accounts_2 | UNIQUE      | UNIQUE (full_path)                                                                                       |

## Indexes

| Name                        | Definition         |
| --------------------------- | ------------------ |
| sqlite_autoindex_accounts_1 | PRIMARY KEY (id)   |
| sqlite_autoindex_accounts_2 | UNIQUE (full_path) |

## Relations

```mermaid
erDiagram

"accounts" }o--|| "commodities" : "FOREIGN KEY (commodity) REFERENCES commodities (code) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE"
"accounts" }o--|| "commodities" : "accounts.commodity -> commodities.code"
"accounts" }o--o| "customers" : "FOREIGN KEY (customer_id) REFERENCES customers (id) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE"
"accounts" }o--o| "customers" : "accounts.customer_id -> customers.id"
"aliases" }o--|| "accounts" : "FOREIGN KEY (account_path) REFERENCES accounts (full_path) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE"
"aliases" }o--|| "accounts" : "aliases.account_path -> accounts.full_path"
"balances_live" }o--|| "accounts" : "FOREIGN KEY (account_id) REFERENCES accounts (id) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE"
"balances_live" }o--|| "accounts" : "balances_live.account_id -> accounts.id"
"movements" }o--|| "accounts" : "FOREIGN KEY (to_account_id) REFERENCES accounts (id) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE"
"movements" }o--|| "accounts" : "FOREIGN KEY (from_account_id) REFERENCES accounts (id) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE"
"movements" }o--|| "accounts" : "movements.from_account_id -> accounts.id"
"movements" }o--|| "accounts" : "movements.to_account_id -> accounts.id"

"accounts" {
  TEXT account_id "Specific account identifier within the product"
  TEXT account_type "One of: Asset, Liability, Equity, Income, Expense"
  TEXT address "Sub-address within the account (e.g. branch). 'Pending' marks pending accounts"
  TEXT commodity FK "Commodity code (FK to commodities.code)"
  TEXT created_at "Timestamp when the account was created"
  TEXT customer_id FK "Optional owning customer (FK to customers.id). A customer may have many accounts"
  TEXT full_path "Hierarchical account path, e.g. Asset:Bank:Current:Main"
  TEXT gross_interest_rate "Gross annual interest rate as a decimal (0.045 = 4.5%)"
  TEXT id PK "UUID primary key"
  INTEGER interest_accumulator "Sub-unit fractions at extended precision (method-dependent)"
  TEXT interest_method "Interest calculation method (e.g. simple_daily)"
  INTEGER is_pending "True if this is a pending/suspense account"
  TEXT opened_at "When the account was opened"
  TEXT product "Product category within the account type"
}
"commodities" {
  TEXT code "Unique commodity code (e.g. GBP, USD, BTC)"
  TEXT created_at "Timestamp when the commodity was created"
  TEXT datetime "Optional date associated with the commodity definition"
  INTEGER exponent "Decimal exponent for amount precision (-2 = pence, -8 = satoshi)"
  TEXT id PK "UUID primary key"
}
"customers" {
  TEXT created_at "Timestamp when the customer was created"
  TEXT id PK "UUID primary key"
  TEXT max_balance_amount "Maximum allowed balance amount (empty = no limit)"
  TEXT max_balance_commodity "Commodity for the max balance constraint"
  TEXT name "Customer name (unique)"
}
"aliases" {
  TEXT account_path FK "Full account path (FK to accounts.full_path)"
  TEXT created_at "Timestamp when the alias was created"
  TEXT id PK "UUID primary key"
  TEXT name "Alias name (unique)"
}
"balances_live" {
  TEXT account_id FK "Account this balance belongs to (FK to accounts.id)"
  INTEGER balance "End-of-day balance in smallest currency unit"
  TEXT balance_date "Date of the balance snapshot (start of day)"
  TEXT id PK "UUID primary key"
  TEXT updated_at "When this balance was last recomputed"
}
"movements" {
  INTEGER amount "Transfer amount in smallest currency unit (integer at commodity exponent)"
  TEXT batch_id "Groups related movements into a single atomic transaction"
  TEXT code "ISO 20022 BTC mnemonic (DOMAIN:FAMILY:SUBFAMILY)"
  TEXT description "Human-readable description of the movement"
  TEXT from_account_id FK "Source account (FK to accounts.id)"
  TEXT id PK "UUID primary key"
  TEXT knowledge_time "When the system recorded this movement (knowledge date)"
  INTEGER ledger "Partition identifier for multi-ledger setups (TigerBeetle-inspired)"
  INTEGER pending_id "Two-phase commit: references pending movement to post/void (0=N/A)"
  TEXT period_anchor "Period anchor marker: ^ (start), $ (end), or empty"
  TEXT to_account_id FK "Destination account (FK to accounts.id)"
  INTEGER user_data_64 "Arbitrary external reference for application use"
  TEXT value_time "When the movement economically occurred (value date)"
}
```

---

> Generated by [tbls](https://github.com/k1LoW/tbls)
