# Plain text syntax

We are going to borrow from the EBNF format in https://github.com/drummonds/plain-text-accounting-formats



## Chart of Accounts

One of the critical elements in accounting theory is on categorising
accounts so that reports can be produced.

You have more balance type accounts like assets and liabilities.  The 
reason for differentiating them is the sign of them.

An full account can be something like:

Liabilitiy:InterestAccount:0000-111:
eg
Type:Product:AccountID:Address

Each account address can also be :Pending

You should be able to report every day on any level of an account.


## Movements

Pacioli had Credit // Debit 

this replaces transactions and postings

### Single Movements

Allowed variations including the virgolette from Pacioli.

2026-02-07 *
  Cash → Equity 200 GBP

2026-02-07 *
  Cash // Equity 200 GBP

2026-02-07 *
  Cash > Equity 200 GBP

2026-02-07 *
  Cash -> Equity 200 GBP

Including a description

2026-02-07 *
  Cash → Equity "Dividend" 200 GBP


### Linked Movements

Tiger Beatle tags subsequent movements which are related eg

2026-02-07 *
  Cash → Purchases 5
  +Cash → VATInput 1


## Parameters

Not all data is movements