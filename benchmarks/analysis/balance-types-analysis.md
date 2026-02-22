This shows that future date timing has marginal effect on accessing balance information 
for millions of accounts so it  is practical from this point of view.

With the (account_id, value_time DESC) index, all three types deliver sub-millisecond
point-in-time lookups even at 36.5M rows. The data type choice has negligible impact
on indexed single-row retrieval.

Use BIGINT (integer cents/pence) for balance storage. It matches go-luca's existing
int64 amount model, avoids floating-point rounding (DOUBLE), and has no NUMERIC
overhead. Reserve NUMERIC for external reporting views where decimal display formatting
is needed.
