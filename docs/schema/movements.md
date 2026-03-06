# movements

## Description

Core transaction records. Each movement transfers an integer amount from one account to another. Movements with the same batch_id form a linked transaction (compound entry). Inspired by TigerBeetle's transfer model with code, ledger, and pending_id fields.  


<details>
<summary><strong>Table Definition</strong></summary>

```sql
CREATE TABLE movements (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_id INTEGER NOT NULL,
    from_account_id INTEGER NOT NULL,
    to_account_id INTEGER NOT NULL,
    amount INTEGER NOT NULL,
    code INTEGER NOT NULL DEFAULT 0,
    ledger INTEGER NOT NULL DEFAULT 0,
    pending_id INTEGER NOT NULL DEFAULT 0,
    user_data_64 INTEGER NOT NULL DEFAULT 0,
    value_time TEXT NOT NULL,
    knowledge_time TEXT DEFAULT (datetime('now')),
    description TEXT NOT NULL DEFAULT ''
)
```

</details>

## Columns

| Name            | Type    | Default         | Nullable | Children | Parents                 | Comment                                                                 |
| --------------- | ------- | --------------- | -------- | -------- | ----------------------- | ----------------------------------------------------------------------- |
| amount          | INTEGER |                 | false    |          |                         | Transfer amount in smallest currency unit (integer at account exponent) |
| batch_id        | INTEGER |                 | false    |          |                         | Groups related movements into a single atomic transaction               |
| code            | INTEGER | 0               | false    |          |                         | Movement category: 0=normal, 1=interest accrual (TigerBeetle-inspired)  |
| description     | TEXT    | ''              | false    |          |                         | Human-readable description of the movement                              |
| from_account_id | INTEGER |                 | false    |          | [accounts](accounts.md) | Source account (references accounts.id)                                 |
| id              | INTEGER |                 | true     |          |                         | Auto-incrementing primary key                                           |
| knowledge_time  | TEXT    | datetime('now') | true     |          |                         | When the system recorded this movement (knowledge date)                 |
| ledger          | INTEGER | 0               | false    |          |                         | Partition identifier for multi-ledger setups (TigerBeetle-inspired)     |
| pending_id      | INTEGER | 0               | false    |          |                         | Two-phase commit: references pending movement to post/void (0=N/A)      |
| to_account_id   | INTEGER |                 | false    |          | [accounts](accounts.md) | Destination account (references accounts.id)                            |
| user_data_64    | INTEGER | 0               | false    |          |                         | Arbitrary external reference for application use                        |
| value_time      | TEXT    |                 | false    |          |                         | When the movement economically occurred (value date)                    |

## Constraints

| Name | Type        | Definition       |
| ---- | ----------- | ---------------- |
| id   | PRIMARY KEY | PRIMARY KEY (id) |

## Indexes

| Name                | Definition                                                                    |
| ------------------- | ----------------------------------------------------------------------------- |
| idx_movements_batch | CREATE INDEX idx_movements_batch ON movements(batch_id)                       |
| idx_movements_code  | CREATE INDEX idx_movements_code ON movements(to_account_id, code, value_time) |
| idx_movements_from  | CREATE INDEX idx_movements_from ON movements(from_account_id, value_time)     |
| idx_movements_to    | CREATE INDEX idx_movements_to ON movements(to_account_id, value_time)         |

## Relations

```mermaid
erDiagram

"movements" }o--|| "accounts" : "movements.from_account_id -> accounts.id"
"movements" }o--|| "accounts" : "movements.to_account_id -> accounts.id"

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
"accounts" {
  TEXT account_id "Specific account identifier within the product"
  TEXT account_type "One of: Asset, Liability, Equity, Income, Expense"
  TEXT address "Sub-address within the account (e.g. branch). 'Pending' marks pending accounts"
  REAL annual_interest_rate "Annual interest rate as a decimal (0.045 = 4.5%)"
  TEXT created_at "Timestamp when the account was created"
  TEXT currency "ISO 4217 currency code (e.g. GBP, USD)"
  INTEGER exponent "Decimal exponent for amount precision (-2 = pence, -5 = high precision)"
  TEXT full_path "Hierarchical account path, e.g. Asset:Bank:Current:Main"
  INTEGER id "Auto-incrementing primary key"
  INTEGER is_pending "True if this is a pending/suspense account"
  TEXT product "Product category within the account type"
}
```

---

> Generated by [tbls](https://github.com/k1LoW/tbls)
