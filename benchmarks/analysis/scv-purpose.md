FSCS requires firms to produce Single Customer View (SCV) files regularly. This benchmark
measures how fast go-luca can generate the two key CSV files from a PostgreSQL database:

- **C file**: Individual account records (one row per account, pipe-delimited)
- **D file**: Aggregate customer balances with compensatable amounts capped at £85,000

The benchmark scales from 1K to 1M accounts. Customers have 1–100 accounts each with
random balances 0–100K GBP. Products are randomly assigned from IAA, ISA, NA, and FD1.

Each iteration performs a full SCV generation: query all liability accounts with computed
balances, group by customer, write pipe-delimited C and D files with the FSCS footer.
