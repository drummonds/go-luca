# go-luca

This borrows https://www.bytestone.uk/afp/movements/ which explains that cash movements are the fundamental concept not posting legs.

[Luca Paciolo][] taught accounting in the fifteen century and crystallises some very important notions:
- Transfers as the fundamental record of account
- Separation of real time activity (the day book) with historical journal (formalised list of activity for a period of time) and ledgers (denormalised view by account with balances for a period).

[Luca Paciolo](https://www.bytestone.uk/afp/historical-accounting/pacioli/)
# Obects

The overall object is a ledger which contains the other objects.

The ledger is a list of events. Each event has a value time and an optional knowledge time.

A transaction happens over a period of time and includes multiple movement batches that occur at a single point in time.

I also want to cover group accounting which allows you to aggregate smaller entities into larger onces.
So a bank with 10 million customers might be organised into branches with 100,000 customers.


## References 

  [plain text syntax][./TEXT_SYNTAX.md]



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


