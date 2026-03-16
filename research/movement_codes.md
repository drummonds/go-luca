# Movement Type Codes

Research into designing a compact integer movement type code for go-luca,
informed by ISO 20022 Bank Transaction Codes, ISO 8583, TigerBeetle, and
Swedish payment standards.

## 1. Why Movement Codes Matter

Every movement in a double-entry ledger has a *reason*. Without a structured
code the only explanation is a free-text description, which is impossible to
filter, aggregate, or reconcile programmatically. A well-designed code field
answers: "why did this value flow?" — enabling reporting, regulatory
classification, and automated reconciliation.

## 2. ISO 20022 Bank Transaction Codes (BTC)

ISO 20022 uses a **3-level hierarchy** of 4-character alphabetic mnemonics
carried in camt.052/053/054 messages:

    Domain → Family → Sub-Family

There are **no official numeric codes** — the BTC_Codification spreadsheet
has row numbers but these are not stable identifiers.

### 2.1 Domains

| Code | Domain                          |
|------|---------------------------------|
| PMNT | Payments                        |
| CAMT | Cash Management                 |
| LDAS | Loans, Deposits & Syndications  |
| FORX | Foreign Exchange                |
| SECU | Securities                      |
| DERV | Derivatives                     |
| TRAD | Trade Services                  |
| PMET | Precious Metal                  |
| CMDT | Commodities                     |
| ACMT | Account Management              |
| XTND | Extended Domain (catch-all)     |

### 2.2 Payments (PMNT) — Family Codes

| Family | Description                          |
|--------|--------------------------------------|
| RCDT   | Received Credit Transfers            |
| ICDT   | Issued Credit Transfers              |
| RCCN   | Received Cash Concentration          |
| ICCN   | Issued Cash Concentration            |
| RDDT   | Received Direct Debits               |
| IDDT   | Issued Direct Debits                 |
| RCHQ   | Received Cheques                     |
| ICHQ   | Issued Cheques                       |
| CCRD   | Customer Card Transactions           |
| MCRD   | Merchant Card Transactions           |
| LBOX   | Lockbox Transactions                 |
| CNTR   | Counter Transactions (OTC cash)      |
| DRFT   | Drafts / Bills of Exchange           |
| MCOP   | Miscellaneous Credit Operations      |
| MDOP   | Miscellaneous Debit Operations       |

### 2.3 Key Sub-Family Codes (under RCDT/ICDT)

| Sub-Family | Description                        |
|------------|------------------------------------|
| DMCT       | Domestic Credit Transfer           |
| XBCT       | Cross-Border Credit Transfer       |
| ESCT       | SEPA Credit Transfer               |
| SALA       | Salary/Payroll Payment             |
| SDVA       | Same-Day Value Credit Transfer     |
| STDO       | Standing Order                     |
| BOOK       | Internal Book Transfer             |
| AUTT       | Automatic Transfer                 |
| ATXN       | ACH Transaction                    |
| RRTN       | Return/Reimbursement               |

### 2.4 Generic Sub-Family Codes (apply across all domains)

These are the most relevant for internal accounting:

| Sub-Family | Description                        |
|------------|------------------------------------|
| INTR       | Interest                           |
| CHRG       | Charges                            |
| FEES       | Fees                               |
| COMM       | Commission                         |
| TAXE       | Taxes                              |
| RIMB       | Reimbursements                     |
| ADJT       | Adjustments                        |
| OTHR       | Other                              |

### 2.5 Loans, Deposits & Syndications (LDAS) — Families

| Family | Description          |
|--------|----------------------|
| FTLN   | Fixed Term Loans     |
| NTLN   | Notice Loans         |
| FTDP   | Fixed Term Deposits  |
| NTDP   | Notice Deposits      |
| MGLN   | Mortgage Loans       |
| CSLN   | Consumer Loans       |
| SYDN   | Syndications         |

Interest accrual/application: `LDAS > FTDP > INTR` (deposit interest),
`LDAS > MGLN > INTR` (mortgage interest).  Fees: any domain + `FEES`/`CHRG`.

### 2.6 Cash Management (CAMT) — Families

| Family | Description           |
|--------|-----------------------|
| CAPL   | Cash Pooling          |
| ACCB   | Account Balancing     |

Sub-families under ACCB: ZBAL (zero-balancing), SWEP (sweep), TOPG (top-up),
ODRF (overdraft).

### 2.7 ISO 20022 Purpose Codes (ExternalPurpose1Code)

Separate from BTC, carried in payment messages to indicate payment purpose:

| Code | Description                        |
|------|------------------------------------|
| INTE | Interest Payment                   |
| LOAN | Loan to Borrower                   |
| LOAR | Loan Repayment to Lender           |
| DEPT | Deposit                            |
| SALA | Salary Payment                     |
| PENS | Pension Payment                    |
| BONU | Bonus Payment                      |
| DIVD | Dividend                           |
| TAXS | Tax Payment                        |
| VATX | VAT Payment                        |
| INTX | Income Tax                         |
| LICF | Licence Fee                        |
| INVS | Investment & Securities            |
| ACCT | Account Management                 |
| CASH | Cash Management                    |
| NETT | Netting                            |
| INTC | Intra-Company Payment              |

## 3. ISO 8583 Processing Codes

ISO 8583 (card payment messaging) uses a 6-digit DE3 processing code:
digits 1–2 = transaction type, 3–4 = from-account type, 5–6 = to-account type.

### 3.1 Transaction Type (digits 1–2)

| Code | Description                         |
|------|-------------------------------------|
| 00   | Purchase (Goods and Services)       |
| 01   | Cash Advance / ATM Withdrawal       |
| 02   | Debit Adjustment                    |
| 09   | Purchase with Cashback              |
| 20   | Credit Voucher / Refund             |
| 21   | Deposit                             |
| 22   | Credit Adjustment                   |
| 30   | Available Funds Inquiry             |
| 31   | Balance Inquiry                     |
| 40   | Account Transfer                    |
| 50   | Bill Payment                        |

Ranges: 00–19 debits, 20–29 credits, 30–39 inquiries, 40–49 transfers.

### 3.2 Account Type (digits 3–4 and 5–6)

| Code | Description             |
|------|-------------------------|
| 00   | Default / Unspecified   |
| 10   | Savings Account         |
| 20   | Cheque / Current        |
| 30   | Credit Facility         |
| 40   | Universal Account       |
| 50   | Investment Account      |

## 4. Swedish Payment Standards

Sweden is migrating from legacy formats (Bankgirot LB, KI, BgMax) to ISO
20022 XML. The transition is in two phases during 2026:

- **Spring 2026**: Account-to-account credit transfers (salaries, pensions)
- **Autumn 2026**: Bankgiro and Plusgiro payments

The P27 Nordic Payment Platform initiative (SEB, Danske Bank, Handelsbanken,
Nordea, OP Financial Group, Swedbank) was suspended in 2023; Swedish banks now
focus on domestic infrastructure.

### 4.1 Swedish Payment Systems

| System       | Type                 | Settlement | Notes                         |
|--------------|----------------------|------------|-------------------------------|
| Bankgirot    | Giro / mass retail   | Same day   | Since 1959, OCR references    |
| PlusGirot    | Giro (Nordea)        | Same day   | 2–8 digit numbers             |
| Dataclearing | Account-to-account   | Same day   | 4 daily cycles, cut-off 13:30 |
| Swish        | Mobile instant       | Real-time  | 95% adult adoption            |
| Autogiro     | Direct debit (pull)  | Same day   | 80% of Swedes have mandates   |
| RIX-RTGS     | High-value           | Weekday    | Riksbank-operated             |
| RIX-INST     | Instant / 24×7       | Real-time  | Launched 2022                 |

### 4.2 Swedish ISO 20022 Interpretation

Finance Sweden publishes the *Swedish Common Interpretation of ISO 20022
Payment Messages*. Key Swedish category purpose codes:

| Code | Swedish Use              |
|------|--------------------------|
| SALA | Salary (LÖN)            |
| PENS | Pension                  |

These codes affect execution-date rules: banks may modify
`RequestedExecutionDate` for salary/pension payments to follow Swedish
payment-calendar rules.

The Swedish interpretation uses standard ISO 20022 BTC codes in camt.053
statements — there are no Sweden-specific BTC domains or families. The
differentiation is in *which* sub-families are commonly used and how
category purpose codes interact with execution timing.

### 4.3 Relevance to go-luca

Swedish standards don't define alternative transaction type codes — they adopt
ISO 20022 directly. The main takeaway: our code field should be able to
represent ISO 20022 BTC leaf codes so that movements imported from Swedish
(or any ISO 20022) bank statements can retain their classification.

## 5. TigerBeetle's Approach

TigerBeetle uses `uint16` (0–65535) for `Transfer.code`. Their guidance:

- Code `0` means "unset" — cannot be used
- Codes are user-defined enums — no built-in meanings
- Used for filtering and querying transfers by type
- The code is immutable after creation

go-luca currently has `Code int16` on Movement with:
- `CodeNormal = 0` (general movement)
- `CodeInterestAccrual = 1` (interest journal entry)

## 6. Proposed Code Field Design

### 6.1 Text-Based Codes

Use the ISO 20022 mnemonic format directly as a short string:

    DOMAIN:FAMILY:SUBFAMILY

Using `:` as separator — consistent with go-luca's existing account path
convention (`Asset:Bank:Current:Main`).

**Go type:** `Code string` on `Movement` and `MovementInput`.
**DB column:** `VARCHAR(14) NOT NULL` — max 4+1+4+1+4 = 14 chars.
**Compulsory:** Every movement must have a non-empty code.

### 6.2 Why Text Over Packed Integer

A packed uint32 bit layout was considered (see section 8) but rejected:

- **Readability**: `"LDAS:FTDP:INTR"` is self-documenting in DB queries,
  logs, CSV exports. No lookup table needed.
- **ISO 20022 compatibility**: Codes map directly to/from camt.053 BTC
  fields with no numeric encoding/decoding step.
- **Debugging**: `SELECT * FROM movements WHERE code = 'LDAS:FTDP:INTR'`
  vs `WHERE code & 0x1F000000 = 0x03000000`.
- **Storage**: pglike/SQLite stores everything as text anyway. The 14-byte
  overhead vs 4 bytes is irrelevant at go-luca's scale.
- **Simplicity**: No bit-twiddling helpers to write and maintain.

### 6.3 go-luca System Codes

| Code                 | Description                       |
|----------------------|-----------------------------------|
| `PMNT:RCDT:DMCT`     | Received domestic credit transfer |
| `PMNT:RCDT:BOOK`     | Internal book transfer received   |
| `PMNT:ICDT:DMCT`     | Issued domestic credit transfer   |
| `PMNT:ICDT:BOOK`     | Internal book transfer issued     |
| `PMNT:RDDT:OTHR`     | Received direct debit             |
| `PMNT:IDDT:OTHR`     | Issued direct debit               |
| `LDAS:FTDP:INTR`     | Deposit interest accrual          |
| `LDAS:NTDP:INTR`     | Notice deposit interest accrual   |
| `LDAS:FTLN:INTR`     | Loan interest accrual             |
| `LDAS:MGLN:INTR`     | Mortgage interest accrual         |
| `ACMT:MCOP:OTHR`     | Opening balance / adjustment      |
| `ACMT:MDOP:FEES`     | Fee charge                        |
| `ACMT:MDOP:CHRG`     | General charge                    |
| `ACMT:MDOP:ADJT`     | Adjustment                        |
| `ACMT:MDOP:TAXE`     | Tax                               |

### 6.4 Practical Codes for go-luca's Immediate Needs

| Operation                    | Old Code                 | New Code               |
|------------------------------|--------------------------|------------------------|
| Client money in              | (none)                   | `PMNT:RCDT:DMCT`       |
| Client money out             | (none)                   | `PMNT:ICDT:DMCT`       |
| Internal book transfer       | `CodeNormal(0)`          | `PMNT:RCDT:BOOK`       |
| Interest accrual journal     | `CodeInterestAccrual(1)` | `LDAS:FTDP:INTR`       |
| Interest application         | (none)                   | `LDAS:FTDP:OTHR`       |
| Fee charge                   | (none)                   | `ACMT:MDOP:FEES`       |
| Opening balance              | (none)                   | `ACMT:MCOP:OTHR`       |

### 6.5 User-Defined Codes

User codes follow the same `X:Y:Z` format but use a `USER` domain prefix:

    USER:MYAPP:REBATE
    USER:CUST:REFUND

This avoids collision with ISO 20022 domains (no ISO domain is `USER`).

### 6.6 Filtering and Indexing

Text codes support prefix queries naturally:

```sql
-- All interest accruals
SELECT * FROM movements WHERE code LIKE 'LDAS:%:INTR'

-- All payment domain movements
SELECT * FROM movements WHERE code LIKE 'PMNT:%'

-- Exact match
SELECT * FROM movements WHERE code = 'LDAS:FTDP:INTR'
```

The existing index `idx_movements_code ON movements(to_account_id, code, value_time)`
works well with text codes — the composite index supports exact match and
prefix queries efficiently.

## 7. Implementation

### 7.1 Migration from int16 to string

The existing `Code int16` field has only two values in use (0, 1). Migration:

1. Change DB column: `code SMALLINT NOT NULL DEFAULT 0` →
   `code VARCHAR(14) NOT NULL`
2. Change Go type: `Code int16` → `Code string`
3. Map old values: `0 → "PMNT:RCDT:BOOK"`, `1 → "LDAS:FTDP:INTR"`
4. Add `code` parameter to `RecordMovement` and
   `RecordMovementWithProjections` (was previously hardcoded)
5. Validate code is non-empty in all recording methods

### 7.2 Go Constants

```go
const (
    CodeBookTransfer    = "PMNT:RCDT:BOOK"  // internal book transfer
    CodeInterestAccrual = "LDAS:FTDP:INTR"  // deposit interest accrual
    CodeCreditReceived  = "PMNT:RCDT:DMCT"  // received domestic credit transfer
    CodeCreditIssued    = "PMNT:ICDT:DMCT"  // issued domestic credit transfer
    CodeFee             = "ACMT:MDOP:FEES"  // fee charge
    CodeOpeningBalance  = "ACMT:MCOP:OTHR"  // opening balance / adjustment
)
```

### 7.3 Text Format (.goluca files)

Movement codes should be part of the text format specification. The code
can be carried as transaction metadata or as a field on the movement line.
This is a future enhancement — for now, imported transactions without
explicit codes default to `CodeBookTransfer`.

## 8. Alternative Designs Considered

### 8.1 Flat Enum (int16, current approach)

Pros: Simple, TigerBeetle-compatible.
Cons: No structure, no hierarchy, no user space, 32K codes max, opaque.

### 8.2 Packed uint32 (bit-field layout)

Pros: Compact (4 bytes), fast comparisons, bit-level domain filtering.
Cons: Opaque in DB queries, requires encode/decode helpers, lookup tables
for human-readable output. Over-engineered for go-luca's scale.

### 8.3 Two Fields (system_code + user_code)

Pros: Clean separation.
Cons: Extra column, extra index, more complex queries.

## 9. Recommendation

**Use `VARCHAR(14)` text codes in `DOMAIN:FAMILY:SUBFAMILY` format.**

- Human-readable everywhere — DB, logs, exports, debugging
- Direct ISO 20022 BTC compatibility with no mapping layer
- Consistent `:` separator with account path convention
- Compulsory — every movement must explain *why* it exists
- `USER:` prefix convention for application-specific codes
- Simple `LIKE 'DOMAIN:%'` queries for domain filtering

Pending operations remain handled by the existing `PendingID int64` field —
pending is a *state*, not a movement classification.

## 10. Sources

- [ISO 20022 External Code Sets](https://www.iso20022.org/catalogue-messages/additional-content-messages/external-code-sets)
- [ISO 20022 BTC Structure Report](https://www.iso20022.org/sites/default/files/2021-02/BTC_ExternalCodeListDescription_Feb2021.doc)
- [ISO 20022 BTC Codification (Oct 2023)](https://www.iso20022.org/sites/default/files/media/file/BTC_Codification_30October2023.xls)
- [ISO 20022 BTC External Code List (May 2025)](https://www.iso20022.org/sites/default/files/media/file/BTC_ExternalCodeListDescription_May2025_v2.docx)
- [ISO 8583 Processing Codes (neapay)](https://neapay.com/post/iso8583-processing-codes-transaction-processing_98.html)
- [TigerBeetle Transfer Reference](https://docs.tigerbeetle.com/reference/transfer/)
- [Finance Sweden — Swedish Common Interpretation of ISO 20022](https://www.financesweden.se/media/1266/7_appendix_1_common_payment_types_in_sweden.pdf)
- [Finance Sweden — camt.053 Swedish Interpretation](https://www.financesweden.se/media/3287/5-camt05300102-swedish-common-interpretation.pdf)
- [Swedbank — P27 / ISO 20022](https://www.swedbank.com/corporate/cash-management-transaction-services/p27.html)
- [Atlar — Guide to Bank Payments in Sweden](https://www.atlar.com/guides/bank-payments-in-sweden)
- [SEB — New Payment Infrastructure in Sweden](https://sebgroup.com/our-offering/cash-management/cash-management-news/new-payment-infrastructure-in-sweden)
