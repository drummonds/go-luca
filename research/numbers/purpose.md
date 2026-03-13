Reexamine numeric format choices for go-luca's amount representation.

go-luca currently uses int64 in smallest currency unit with Account.Exponent
(TigerBeetle-inspired). shopspring/decimal is used for display and interest
calculations. This benchmark evaluates whether the format choices remain optimal
by comparing all major Go decimal/money libraries across performance, precision,
database compatibility, and ecosystem support.

Key questions:
- Is shopspring/decimal still the right choice for calculations, or is govalues/decimal better?
- How do the libraries integrate with pgx for PostgreSQL NUMERIC columns?
- How do they round-trip through pglike's SQLite backend (where NUMERIC currently becomes REAL)?
- Should pglike translate NUMERIC to TEXT instead of REAL to preserve precision?
- Does the int64+exponent core model remain sound vs arbitrary-precision alternatives?
