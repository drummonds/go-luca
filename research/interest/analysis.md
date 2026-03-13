# Interest Calculation Design

## 1. AER and Gross Rates

**AER (Annual Equivalent Rate)** is the effective annual return including the effect of compounding. It is the standard comparison rate quoted to consumers (FCA BCOBS). A **gross rate** is the simple annual rate before tax, which may or may not equal AER depending on compounding frequency.

The relationship:

```
AER = (1 + gross_rate / n)^n - 1
```

where `n` is the number of compounding periods per year. When interest is compounded daily (`n = 365` or `366`):

```
AER = (1 + gross_daily_rate)^365 - 1
```

Conversely, to derive the gross daily rate from an AER:

```
gross_daily_rate = (1 + AER)^(1/365) - 1
```

For small rates this is very close to `AER / 365`, but the difference matters over long periods and for regulatory accuracy.

**Should systems quote gross daily rates?** Generally no. Products are quoted as AER or gross annual rate. The daily rate is an internal calculation detail. However the *precision* of the internally derived daily rate is critical (see Section 4).

## 2. Day-Count Conventions

Day-count conventions determine how many days of interest accrue over a period. The two most relevant for UK banking:

### Actual/365 (fixed)

- Numerator: actual number of calendar days in the period.
- Denominator: always 365, regardless of leap years.
- Used by most UK banks for savings and loan interest.
- In a leap year, a full year accrues 366/365 of the annual rate — the customer earns slightly more.
- Simple and predictable. No special leap-year logic needed in the daily engine.

### Actual/Actual (ISDA)

- Numerator: actual days.
- Denominator: 365 in a non-leap year, 366 in a leap year (or pro-rated if the period spans both).
- Used in some bond markets and interbank lending (ISDA conventions).
- A full year always accrues exactly the annual rate, regardless of leap years.
- More complex: when a period spans a year boundary (e.g. 28 Dec – 3 Jan across a leap/non-leap transition), days must be split and weighted.

### Leap year implications

| Convention | Daily rate (non-leap) | Daily rate (leap) | Full year interest |
|---|---|---|---|
| Actual/365 | rate / 365 | rate / 365 | 366/365 * rate ≈ 1.00274 * rate |
| Actual/Actual | rate / 365 | rate / 366 | exactly rate |

For go-luca, **Actual/365 (fixed)** is the right default — it matches UK retail banking practice, requires no leap-year branching in the daily engine, and is the convention behind the discrete formula in Section 4.

## 3. Daily Interest and AER Comparison

When interest is calculated daily and added to the balance (compounded daily), the effective rate over a year exceeds the gross rate due to compounding.

Example: 4.00% gross, Actual/365, non-leap year.

```
Daily rate     = 0.04 / 365 = 0.00010958904...
Daily interest = balance * daily_rate
```

After 365 days of compounding on £10,000.00:

```
Final balance = 10000 * (1 + 0.04/365)^365 = 10,408.08
Effective AER = 4.0808%
```

The gap between gross rate and AER grows with the rate:

| Gross rate | AER (daily compound) | Difference |
|---|---|---|
| 0.01% | 0.01% | < 0.001% |
| 1.00% | 1.005% | 0.005% |
| 4.00% | 4.081% | 0.081% |
| 10.00% | 10.516% | 0.516% |

### Rounding limits

The core problem: at 2-decimal-place precision (pence), daily interest on small balances rounds to zero.

- £1.00 at 4.00% gross: daily interest = £0.000109589... → rounds to £0.00.
- The balance **never grows**. Interest is silently lost.

This is the "rounding trap" — it is not a numerical curiosity but a fairness issue. A customer with £1 in a savings account earning 4% should eventually see interest credited.

**Threshold for 1p daily interest at 2dp:**

```
balance * rate / 365 >= 0.005  (rounds up to 0.01)
balance >= 0.005 * 365 / rate
```

At 4% gross: balance >= £45.63. Below this, daily interest rounds to zero every day forever.

### The 10,000-year test

At 0.01% gross on 1p (£0.01), using exact arithmetic:

```
0.01 * 0.0001 / 365 = 0.00000000027397...
```

At 2dp, this rounds to £0.00 — no interest ever accrues. Even at 4dp (£0.0001 precision), daily interest = £0.0000000027... which still rounds to zero.

To earn 1p of interest on 1p at 0.01% with no rounding loss requires continuous/exact accumulation over ~10,000 years:

```
0.01 / (0.01 * 0.0001 / 365) ≈ 3,650,000 days ≈ 10,000 years
```

This motivates the need for an accumulation method that preserves fractional interest across days.

## 4. Discrete Interest Formula

### The problem with naive daily rounding

If we round interest to the account's exponent (e.g. 2dp) each day, small balances lose interest permanently. If we use arbitrary-precision decimals, we lose the uniformity and auditability of integer arithmetic.

### The intermediate-precision approach

Use 4-digit precision (exponent -4, i.e. hundredths of a penny) as the **interest accumulator**, while keeping account balances at their native exponent (e.g. -2 for GBP).

The formula for daily interest at 4-digit precision, using only integer arithmetic:

```
interest_4dp = (daily_amount * num_days * gross_rate_scaled) / divisor
```

where:

```
gross_rate_scaled = gross_annual_rate * 365 * 100000    (integer)
divisor           = 365 * 10000                         (integer, = 3650000)
```

Worked example: £100.00 at 4.00% gross, 1 day.

```
daily_amount      = 10000        (£100.00 in pence, at exponent -2)
                  = 1000000      (at exponent -4: 10000 * 100)
num_days          = 1
gross_rate_scaled = 0.04 * 365 * 100000 = 1460000
divisor           = 365 * 10000 = 3650000

interest_4dp = (1000000 * 1 * 1460000) / 3650000
             = 1460000000000 / 3650000
             = 400000
             → £0.0400 per day (i.e. 400000 at exponent -6? No...)
```

Let's restate more carefully. Express amounts in the 4dp unit (1 unit = £0.0001):

```
balance_4dp       = 1000000      (£100.00 = 1,000,000 units of £0.0001)
gross_rate_int    = 4000         (4.00% expressed as rate * 100000)
daily_interest_4dp = balance_4dp * gross_rate_int / (365 * 100000)
                   = 1000000 * 4000 / 36500000
                   = 4000000000 / 36500000
                   = 109 (truncated)
                   → £0.0109 per day
```

Check: £100.00 * 0.04 / 365 = £0.01095890... → at 4dp this is £0.0109 (truncated) or £0.0110 (rounded). The integer formula gives 109 units of £0.0001 = £0.0109. Correct.

### The accumulator pattern

Each account maintains an **interest accumulator** at exponent -4 (or whatever extended precision is chosen). Each day:

1. Compute `daily_interest_4dp` using integer arithmetic.
2. Add to the accumulator: `accumulator += daily_interest_4dp`.
3. When `accumulator >= rounding_threshold` (e.g. 50 units of £0.0001 = £0.005, the half-penny that rounds up), extract whole pence and post as a movement.

Alternatively, accumulate for a fixed period (monthly, quarterly) and round once at posting.

### Small balance behaviour

£0.01 at 0.01% gross, per day:

```
balance_4dp    = 100          (£0.01 = 100 units of £0.0001)
gross_rate_int = 10           (0.01% * 100000)
daily_4dp      = 100 * 10 / 36500000 = 0 (truncated)
```

Even at 4dp, £0.01 at 0.01% produces zero daily interest. The accumulator stays at zero. This is mathematically correct — the amount is so small and the rate so low that even 4dp cannot represent it.

To handle this extreme case, either:
- Accept it (£0.01 at 0.01% = £0.000001/year — below any reasonable materiality threshold).
- Use 6dp or 8dp accumulators for extreme precision (but adds complexity for negligible real-world benefit).

For practical rates (>= 0.1%) and practical balances (>= £1.00):

```
balance_4dp    = 10000        (£1.00)
gross_rate_int = 100          (0.1%)
daily_4dp      = 10000 * 100 / 36500000 = 0 (still zero)

balance_4dp    = 100000       (£10.00)
gross_rate_int = 100          (0.1%)
daily_4dp      = 100000 * 100 / 36500000 = 0 (still zero)

balance_4dp    = 100000       (£10.00)
gross_rate_int = 4000         (4.00%)
daily_4dp      = 100000 * 4000 / 36500000 = 10
               → £0.0010/day. Accumulates to £0.01 in 10 days. ✓
```

At 4dp, the minimum balance to accrue non-zero daily interest at 4.00% is:

```
balance_4dp * 4000 / 36500000 >= 1
balance_4dp >= 36500000 / 4000 = 9125
→ £0.9125, i.e. about 91p
```

This is a substantial improvement over the 2dp threshold of £45.63.

### Properties of this approach

- **Uniform**: same formula for every account, every day. No special cases.
- **Fair**: fractional interest accumulates rather than being silently discarded.
- **Integer-only**: no floating point in the hot path. Deterministic, auditable, reproducible.
- **Works with small values**: the 4dp accumulator captures interest that 2dp would lose.
- **Carries forward**: the accumulator persists across days, so sub-unit interest is never lost (only deferred until it's large enough to round).
- **Discrete**: at any point, the accumulator contains an exact integer. No representation error.

### Bank practice: adjusting the gross rate

Banks sometimes achieve the same effect differently: they adjust the quoted gross rate slightly so that daily interest at 2dp precision matches the target AER over a year. This avoids accumulators but means:

- The effective rate varies by balance band (since rounding effects differ).
- Very small balances still earn nothing.
- The adjusted rate must be recalculated whenever the base rate changes.

The accumulator approach is more principled and treats all customers uniformly.

## 5. Methods of Calculating Interest

### Method 1: Simple daily (current go-luca)

```
interest = balance * annual_rate / 365
```

Round to account precision each day. Post as movement. Subject to the rounding trap for small balances.

### Method 2: Daily with extended-precision accumulator (recommended)

```
daily_interest_4dp = balance_4dp * rate_int / (365 * 100000)
accumulator += daily_interest_4dp
```

Post to account when accumulator exceeds rounding threshold. Eliminates the rounding trap for all practical balances and rates.

### Method 3: Product-of-sums (batch)

For Actual/365:

```
interest = SUM(daily_balance * num_days_at_that_balance) * annual_rate / 365
```

Compute the weighted sum over a period (e.g. a month), then round once. Minimises rounding events. Commonly used in bank batch processing. Compatible with the 4dp accumulator approach.

### Method 4: Continuous compounding

```
interest = balance * (e^(rate * days/365) - 1)
```

Theoretical maximum. Not used in retail banking. Included for completeness.

## 6. Recommendations for go-luca

1. **Adopt the 4dp accumulator** as the core interest engine. Store the accumulator as a field on the account (or a separate interest_accrual table) at exponent -4.

2. **Keep Actual/365 (fixed)** as the default day-count convention. Support Actual/Actual as an option for bond/interbank use cases.

3. **Derive daily rate from gross annual rate** using integer arithmetic as shown in Section 4. Do not store or quote daily rates.

4. **Post accumulated interest** on a configurable schedule (daily/monthly/quarterly/annually). The posting rounds the accumulator to account precision and records a movement.

5. **The interest rate field** (`annual_interest_rate` on Account) should remain the gross annual rate. AER is a derived/display value computed from the gross rate and compounding frequency.

6. **Validate with the benchmark**: run 10,000 years at 0.01% on 1p — the accumulator should eventually reach 1p without loss (at sufficient accumulator precision), confirming no systematic rounding drain.
