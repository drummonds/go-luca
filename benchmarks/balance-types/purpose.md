This is a simulation of realtime balance lookup without caching. In a bank the most
time critical element is movement in of money and then movement out of money. This is
where this query starts to be realistic.

This started out testing summing  three PostgreSQL data types (BIGINT, DOUBLE PRECISION, NUMERIC) for calculated balances or sums.  However it is clear that sum is about 40-50% slower for NUMERIC and the other two are about the same speed.  

This is a test of how much it slows down a calculation if you have live balances with only a single row or you have to query with a date.  The idea is that there will be a balance with todays date and balance with tomorrows date.  As the query rolls over midnight you will return different data due to future dated postings.   I want a realistic test with up to a million accounts in the table.
