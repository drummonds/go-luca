# Numeric Format Choices for Amount Representation

Reexamine numeric format choices for go-luca's amount representation.

go-luca currently uses int64 in smallest currency unit with Account.Exponent
(TigerBeetle-inspired). shopspring/decimal is used for display and interest
calculations. This research evaluates whether the format choices remain optimal
by comparing all major Go decimal/money libraries across performance, precision,
database compatibility, and ecosystem support.

Key questions:
- Is shopspring/decimal still the right choice for calculations, or is govalues/decimal better?
- How do the libraries integrate with pgx for PostgreSQL NUMERIC columns?
- How do they round-trip through pglike's SQLite backend (where NUMERIC currently becomes REAL)?
- Should pglike translate NUMERIC to TEXT instead of REAL to preserve precision?
- Does the int64+exponent core model remain sound vs arbitrary-precision alternatives?

## Go Decimal/Money Library Comparison

| Library | Stars | Internal Type | Precision | Immutable | pgx Adapter | SQLite/pglike | License | Status |
|---|---|---|---|---|---|---|---|---|
| [shopspring/decimal](https://github.com/shopspring/decimal) | ~7,300 | big.Int + int32 exp | Arbitrary | Yes | Official ([jackc/pgx-shopspring-decimal](https://github.com/jackc/pgx-shopspring-decimal)) | TEXT via Valuer/Scanner | MIT | Slow maintenance |
| [cockroachdb/apd](https://github.com/cockroachdb/apd) | ~780 | BigInt + exp + Context | Arbitrary (configurable) | No | None official | TEXT via Valuer/Scanner | Apache-2.0 | Stable, low-freq |
| [alpacahq/alpacadecimal](https://github.com/alpacahq/alpacadecimal) | ~53 | int64 fixed 12dp (fallback: shopspring) | 12 digits / +/-9.2M | Yes | Via shopspring adapter | TEXT via Valuer/Scanner | MIT | Niche, low activity |
| [ericlagergren/decimal](https://github.com/ericlagergren/decimal) | ~577 | uint64 / big.Int modes | Arbitrary | No | None | TEXT via string | BSD-3 | Dormant |
| [govalues/decimal](https://github.com/govalues/decimal) | ~224 | uint64 + scale | 19 significant digits | Yes | Community ([ColeBurch/pgx-govalues-decimal](https://github.com/ColeBurch/pgx-govalues-decimal)) | TEXT via Valuer/Scanner | MIT | Active |
| [govalues/money](https://github.com/govalues/money) | ~49 | govalues/decimal + ISO 4217 | 19 digits | Yes | Via govalues/decimal | TEXT via Valuer/Scanner | MIT | Active |
| [Rhymond/go-money](https://github.com/Rhymond/go-money) | ~1,900 | int64 (minor units) | Currency minor units only | Yes | None | INTEGER | MIT | Flagged inactive |
| [bojanz/currency](https://github.com/bojanz/currency) | ~627 | apd (wrapped) + currency code | Arbitrary | Yes | Via sql interfaces | TEXT via Valuer/Scanner | MIT | Active (CLDR v48.1) |

### Performance ranking (from govalues benchmarks)

govalues/decimal > alpacadecimal > cockroachdb/apd > shopspring/decimal

- govalues: 3-8x faster than shopspring, zero heap allocations for typical ops
- alpacadecimal: 5-100x faster than shopspring within int64 range, falls back to shopspring on overflow
- apd: ~1.5-3.5x faster than shopspring

### pgx NUMERIC integration

pgx's built-in `pgtype.Numeric` can scan to `float64` or `string`. For proper decimal support:

- **shopspring**: Official `jackc/pgx-shopspring-decimal` — register via `pgxdecimal.Register(conn.TypeMap())`
- **govalues**: Community adapter exists (v0.1.0, Feb 2026)
- **apd**: No adapter. Would need custom `NumericCodec`. CockroachDB uses apd natively through its own driver path.
- **ericlagergren**: No adapter

### pglike NUMERIC to SQLite translation (CRITICAL)

In `go-postgres/translate_ddl.go`:

```go
case "NUMERIC", "DECIMAL":
    out = append(out, Token{Kind: TokKeyword, Value: "REAL", Raw: "REAL"})
```

**This loses precision.** SQLite REAL is IEEE 754 double (~15-16 significant digits). PostgreSQL NUMERIC supports up to 131,072 digits before and 16,383 after the decimal.

**Fix needed**: Translate `NUMERIC(p,s)` to `TEXT` instead of `REAL`. All Go decimal libraries round-trip losslessly through text strings. This matches how pglike already handles `TIMESTAMP` to `TEXT`.

SQLite's NUMERIC affinity is also unsafe — it tries to coerce text to INTEGER or REAL. TEXT affinity is the only safe choice for exact decimal storage.

### Approach comparison for go-luca

| Approach | Representation | Pros | Cons |
|---|---|---|---|
| **int64 + exponent** (current) | 10050 with exp=-2 = 100.50 | Fastest. No allocations. Exact. Native SQL INTEGER. | Fixed range (~18 digits). Cross-exponent ops need scaling. |
| **shopspring/decimal** | big.Int + exp | Arbitrary precision. De facto standard. Official pgx adapter. | Slow (big.Int allocations every op). |
| **govalues/decimal** | uint64 + scale | Fast (3-8x shopspring). Immutable. 19 digits. Zero alloc. | Smaller community. 19-digit limit. Community pgx adapter only. |
| **int64 core + govalues display** | int64 storage, govalues for calc/display | Best of both: fast storage, fast calculation | Two representations to bridge |

### The "coin" approach (mkobetic/coin)

Uses `*big.Int + *Commodity` where Commodity defines decimal places. Truncates at every step to commodity precision. Conceptually identical to go-luca's `int64 + Account.Exponent` but with big.Int (arbitrary range, slower).

## Recommendation

1. **Keep int64 + exponent for storage and core movement recording** — it's the fastest, most compact representation and matches TigerBeetle's design
2. **Consider govalues/decimal to replace shopspring/decimal** for interest calculations and display — 3-8x faster, immutable, zero-allocation, 19 digits covers all practical financial amounts
3. **Fix pglike**: change NUMERIC translation from REAL to TEXT to preserve precision
4. **Benchmark the switchover**: measure shopspring vs govalues for go-luca's actual interest calculation workload before committing
