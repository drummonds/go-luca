When funds arrive, two approaches exist:

1. **Simple**: Record the movement, compute balances and interest on demand (or in
   batch at end of day via `RunDailyInterest`). End-of-day processing must recalculate
   everything.

2. **Compound**: At write time, in a single transaction, also pre-compute the interest
   accrual and live balance for end of day. Rollover is seamless — everything is
   already projected.

The benchmark compares throughput of both approaches on real PostgreSQL to quantify the
write-time cost of eagerly projecting interest and balances. The question: is the
compound approach fast enough to use on the hot path?
