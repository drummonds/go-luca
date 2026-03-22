# movements

## Description

Core transaction records. Each movement transfers an integer amount from one account to another. Movements with the same batch_id form a linked transaction (compound entry). Inspired by TigerBeetle's transfer model with code, ledger, and pending_id fields.  


<details>
<summary><strong>Table Definition</strong></summary>

```sql
CREATE TABLE movements (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL,
    from_account_id TEXT NOT NULL REFERENCES accounts(id),
    to_account_id TEXT NOT NULL REFERENCES accounts(id),
    amount INTEGER NOT NULL,
    code TEXT NOT NULL,
    ledger INTEGER NOT NULL DEFAULT 0,
    pending_id INTEGER NOT NULL DEFAULT 0,
    user_data_64 INTEGER NOT NULL DEFAULT 0,
    value_time TEXT NOT NULL,
    knowledge_time TEXT DEFAULT (datetime('now')),
    description TEXT NOT NULL DEFAULT '',
    period_anchor TEXT NOT NULL DEFAULT ''
)
```

</details>

## Columns

| Name            | Type    | Default         | Nullable | Children | Parents                 | Comment                                                                   |
| --------------- | ------- | --------------- | -------- | -------- | ----------------------- | ------------------------------------------------------------------------- |
| amount          | INTEGER |                 | false    |          |                         | Transfer amount in smallest currency unit (integer at commodity exponent) |
| batch_id        | TEXT    |                 | false    |          |                         | Groups related movements into a single atomic transaction                 |
| code            | TEXT    |                 | false    |          |                         | ISO 20022 BTC mnemonic (DOMAIN:FAMILY:SUBFAMILY)                          |
| description     | TEXT    | ''              | false    |          |                         | Human-readable description of the movement                                |
| from_account_id | TEXT    |                 | false    |          | [accounts](accounts.md) | Source account (FK to accounts.id)                                        |
| id              | TEXT    |                 | true     |          |                         | UUID primary key                                                          |
| knowledge_time  | TEXT    | datetime('now') | true     |          |                         | When the system recorded this movement (knowledge date)                   |
| ledger          | INTEGER | 0               | false    |          |                         | Partition identifier for multi-ledger setups (TigerBeetle-inspired)       |
| pending_id      | INTEGER | 0               | false    |          |                         | Two-phase commit: references pending movement to post/void (0=N/A)        |
| period_anchor   | TEXT    | ''              | false    |          |                         | Period anchor marker: ^ (start), $ (end), or empty                        |
| to_account_id   | TEXT    |                 | false    |          | [accounts](accounts.md) | Destination account (FK to accounts.id)                                   |
| user_data_64    | INTEGER | 0               | false    |          |                         | Arbitrary external reference for application use                          |
| value_time      | TEXT    |                 | false    |          |                         | When the movement economically occurred (value date)                      |

## Constraints

| Name                         | Type        | Definition                                                                                                |
| ---------------------------- | ----------- | --------------------------------------------------------------------------------------------------------- |
| - (Foreign key ID: 0)        | FOREIGN KEY | FOREIGN KEY (to_account_id) REFERENCES accounts (id) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE   |
| - (Foreign key ID: 1)        | FOREIGN KEY | FOREIGN KEY (from_account_id) REFERENCES accounts (id) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE |
| id                           | PRIMARY KEY | PRIMARY KEY (id)                                                                                          |
| sqlite_autoindex_movements_1 | PRIMARY KEY | PRIMARY KEY (id)                                                                                          |

## Indexes

| Name                         | Definition                                                                    |
| ---------------------------- | ----------------------------------------------------------------------------- |
| idx_movements_batch          | CREATE INDEX idx_movements_batch ON movements(batch_id)                       |
| idx_movements_code           | CREATE INDEX idx_movements_code ON movements(to_account_id, code, value_time) |
| idx_movements_from           | CREATE INDEX idx_movements_from ON movements(from_account_id, value_time)     |
| idx_movements_to             | CREATE INDEX idx_movements_to ON movements(to_account_id, value_time)         |
| sqlite_autoindex_movements_1 | PRIMARY KEY (id)                                                              |

## Relations

```mermaid
erDiagram

"movements" }o--|| "accounts" : "FOREIGN KEY (from_account_id) REFERENCES accounts (id) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE"
"movements" }o--|| "accounts" : "movements.from_account_id -> accounts.id"
"movements" }o--|| "accounts" : "FOREIGN KEY (to_account_id) REFERENCES accounts (id) ON UPDATE NO ACTION ON DELETE NO ACTION MATCH NONE"
"movements" }o--|| "accounts" : "movements.to_account_id -> accounts.id"

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
```

---

> Generated by [tbls](https://github.com/k1LoW/tbls)
