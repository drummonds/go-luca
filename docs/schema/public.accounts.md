# public.accounts

## Description

Chart of accounts. Each account has a hierarchical path (Type:Product:AccountID:Address) and belongs to one of five fundamental types: Asset, Liability, Equity, Income, Expense. An account optionally belongs to a customer (many accounts per customer). Amounts are stored as integers at the precision defined by the commodity's exponent.  


## Columns

| Name                 | Type                        | Default                  | Nullable | Children                                                                                | Parents                                     | Comment                                                                          |
| -------------------- | --------------------------- | ------------------------ | -------- | --------------------------------------------------------------------------------------- | ------------------------------------------- | -------------------------------------------------------------------------------- |
| account_id           | varchar(100)                | ''::character varying    | false    |                                                                                         |                                             | Specific account identifier within the product                                   |
| account_type         | varchar(50)                 |                          | false    |                                                                                         |                                             | One of: Asset, Liability, Equity, Income, Expense                                |
| address              | varchar(100)                | ''::character varying    | false    |                                                                                         |                                             | Sub-address within the account (e.g. branch). 'Pending' marks pending accounts   |
| commodity            | varchar(50)                 | 'GBP'::character varying | false    |                                                                                         | [public.commodities](public.commodities.md) | Commodity code (FK to commodities.code)                                          |
| created_at           | timestamp without time zone | now()                    | true     |                                                                                         |                                             | Timestamp when the account was created                                           |
| customer_id          | text                        |                          | true     |                                                                                         | [public.customers](public.customers.md)     | Optional owning customer (FK to customers.id). A customer may have many accounts |
| full_path            | varchar(500)                |                          | false    | [public.aliases](public.aliases.md)                                                     |                                             | Hierarchical account path, e.g. Asset:Bank:Current:Main                          |
| gross_interest_rate  | numeric(10,6)               | 0                        | false    |                                                                                         |                                             | Gross annual interest rate as a decimal (0.045 = 4.5%)                           |
| id                   | text                        |                          | false    | [public.balances_live](public.balances_live.md) [public.movements](public.movements.md) |                                             | UUID primary key                                                                 |
| interest_accumulator | bigint                      | 0                        | false    |                                                                                         |                                             | Sub-unit fractions at extended precision (method-dependent)                      |
| interest_method      | varchar(20)                 | ''::character varying    | false    |                                                                                         |                                             | Interest calculation method (e.g. simple_daily)                                  |
| is_pending           | boolean                     | false                    | true     |                                                                                         |                                             | True if this is a pending/suspense account                                       |
| opened_at            | timestamp without time zone |                          | true     |                                                                                         |                                             | When the account was opened                                                      |
| product              | varchar(100)                | ''::character varying    | false    |                                                                                         |                                             | Product category within the account type                                         |

## Constraints

| Name                      | Type        | Definition                                           |
| ------------------------- | ----------- | ---------------------------------------------------- |
| accounts_commodity_fkey   | FOREIGN KEY | FOREIGN KEY (commodity) REFERENCES commodities(code) |
| accounts_customer_id_fkey | FOREIGN KEY | FOREIGN KEY (customer_id) REFERENCES customers(id)   |
| accounts_full_path_key    | UNIQUE      | UNIQUE (full_path)                                   |
| accounts_pkey             | PRIMARY KEY | PRIMARY KEY (id)                                     |

## Indexes

| Name                   | Definition                                                                            |
| ---------------------- | ------------------------------------------------------------------------------------- |
| accounts_full_path_key | CREATE UNIQUE INDEX accounts_full_path_key ON public.accounts USING btree (full_path) |
| accounts_pkey          | CREATE UNIQUE INDEX accounts_pkey ON public.accounts USING btree (id)                 |

## Relations

```mermaid
erDiagram

"public.accounts" }o--|| "public.commodities" : "FOREIGN KEY (commodity) REFERENCES commodities(code)"
"public.accounts" }o--o| "public.customers" : "FOREIGN KEY (customer_id) REFERENCES customers(id)"
"public.aliases" }o--|| "public.accounts" : "FOREIGN KEY (account_path) REFERENCES accounts(full_path)"
"public.balances_live" }o--|| "public.accounts" : "FOREIGN KEY (account_id) REFERENCES accounts(id)"
"public.movements" }o--|| "public.accounts" : "FOREIGN KEY (from_account_id) REFERENCES accounts(id)"
"public.movements" }o--|| "public.accounts" : "FOREIGN KEY (to_account_id) REFERENCES accounts(id)"

"public.accounts" {
  varchar_100_ account_id "Specific account identifier within the product"
  varchar_50_ account_type "One of: Asset, Liability, Equity, Income, Expense"
  varchar_100_ address "Sub-address within the account (e.g. branch). 'Pending' marks pending accounts"
  varchar_50_ commodity FK "Commodity code (FK to commodities.code)"
  timestamp_without_time_zone created_at "Timestamp when the account was created"
  text customer_id FK "Optional owning customer (FK to customers.id). A customer may have many accounts"
  varchar_500_ full_path "Hierarchical account path, e.g. Asset:Bank:Current:Main"
  numeric_10_6_ gross_interest_rate "Gross annual interest rate as a decimal (0.045 = 4.5%)"
  text id "UUID primary key"
  bigint interest_accumulator "Sub-unit fractions at extended precision (method-dependent)"
  varchar_20_ interest_method "Interest calculation method (e.g. simple_daily)"
  boolean is_pending "True if this is a pending/suspense account"
  timestamp_without_time_zone opened_at "When the account was opened"
  varchar_100_ product "Product category within the account type"
}
"public.commodities" {
  varchar_50_ code "Unique commodity code (e.g. GBP, USD, BTC)"
  timestamp_without_time_zone created_at "Timestamp when the commodity was created"
  timestamp_without_time_zone datetime "Optional date associated with the commodity definition"
  integer exponent "Decimal exponent for amount precision (-2 = pence, -8 = satoshi)"
  text id "UUID primary key"
  bigint unit ""
}
"public.customers" {
  timestamp_without_time_zone created_at "Timestamp when the customer was created"
  text id "UUID primary key"
  varchar_50_ max_balance_amount "Maximum allowed balance amount (empty = no limit)"
  varchar_50_ max_balance_commodity "Commodity for the max balance constraint"
  varchar_200_ name "Customer name (unique)"
}
"public.aliases" {
  varchar_500_ account_path FK "Full account path (FK to accounts.full_path)"
  timestamp_without_time_zone created_at "Timestamp when the alias was created"
  text id "UUID primary key"
  varchar_200_ name "Alias name (unique)"
}
"public.balances_live" {
  text account_id FK "Account this balance belongs to (FK to accounts.id)"
  bigint accrued_den ""
  bigint accrued_num ""
  bigint balance "End-of-day balance in smallest currency unit"
  timestamp_without_time_zone balance_date "Date of the balance snapshot (start of day)"
  text id "UUID primary key"
  timestamp_without_time_zone next_day ""
  timestamp_without_time_zone updated_at "When this balance was last recomputed"
}
"public.movements" {
  bigint amount "Transfer amount in smallest currency unit (integer at commodity exponent)"
  text batch_id "Groups related movements into a single atomic transaction"
  varchar_14_ code "ISO 20022 BTC mnemonic (DOMAIN:FAMILY:SUBFAMILY)"
  varchar_500_ description "Human-readable description of the movement"
  text from_account_id FK "Source account (FK to accounts.id)"
  text id "UUID primary key"
  timestamp_without_time_zone knowledge_time "When the system recorded this movement (knowledge date)"
  integer ledger "Partition identifier for multi-ledger setups (TigerBeetle-inspired)"
  bigint pending_id "Two-phase commit: references pending movement to post/void (0=N/A)"
  varchar_1_ period_anchor "Period anchor marker: ^ (start), $ (end), or empty"
  text to_account_id FK "Destination account (FK to accounts.id)"
  bigint user_data_64 "Arbitrary external reference for application use"
  timestamp_without_time_zone value_time "When the movement economically occurred (value date)"
}
```

---

> Generated by [tbls](https://github.com/k1LoW/tbls)
