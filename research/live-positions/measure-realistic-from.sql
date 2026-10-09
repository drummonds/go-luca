\timing off
SET statement_timeout = '300s';
SET jit = off;
\echo === A. as before but the day table analysed
ANALYZE ledger_day;
EXPLAIN (ANALYZE, SUMMARY, TIMING OFF) SELECT account_id, balance, accrued FROM live_business_day;

\echo === B. business day through STABLE functions
CREATE OR REPLACE FUNCTION business_day() RETURNS TIMESTAMP LANGUAGE sql STABLE AS 'SELECT day FROM ledger_day';
CREATE OR REPLACE FUNCTION business_prev_day() RETURNS TIMESTAMP LANGUAGE sql STABLE AS 'SELECT prev_day FROM ledger_day';
CREATE OR REPLACE VIEW live_business_day_fn AS
    SELECT l.account_id, l.full_path, l.commodity,
           round((l.balance
               + COALESCE((SELECT SUM(m.amount) FROM movements m
                           WHERE m.to_account_id = l.account_id AND m.value_time >= l.next_day), 0)
               - COALESCE((SELECT SUM(m.amount) FROM movements m
                           WHERE m.from_account_id = l.account_id AND m.value_time >= l.next_day), 0)
               )::numeric / l.unit, -l.exponent) AS balance,
           round(l.accrued_num::numeric / l.accrued_den / l.unit, 7) AS accrued
    FROM (
        SELECT a.id AS account_id, a.full_path, a.commodity, c.unit, c.exponent,
               COALESCE(p.balance, y.balance,
                        (SELECT z.balance FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1), 0) AS balance,
               COALESCE(p.next_day, y.next_day,
                        (SELECT z.next_day FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1), '0001-01-01') AS next_day,
               COALESCE(p.accrued_num, y.accrued_num, 0) AS accrued_num,
               COALESCE(p.accrued_den, y.accrued_den, 1) AS accrued_den
        FROM accounts a
        JOIN commodities c ON c.code = a.commodity
        LEFT JOIN balances_live p ON p.account_id = a.id AND p.balance_date = business_day()
        LEFT JOIN balances_live y ON y.account_id = a.id AND y.balance_date = business_prev_day()
    ) l;
EXPLAIN (ANALYZE, SUMMARY, TIMING OFF) SELECT account_id, balance, accrued FROM live_business_day_fn;

\echo === C. realistic from-side: withdrawals from a quarter of accounts after their projection
INSERT INTO movements (id, batch_id, from_account_id, to_account_id, amount, code, value_time, description)
  SELECT gen_random_uuid()::text, gen_random_uuid()::text, a.id, e.id, 125, 'WDR', '2025-07-20 11:00', 'withdrawal'
  FROM accounts a, (SELECT id FROM accounts WHERE full_path = 'Equity:Capital') e
  WHERE a.account_type = 'Liability' AND a.account_id::int % 4 = 2;
ANALYZE movements;
SELECT attname, n_distinct FROM pg_stats WHERE tablename = 'movements' AND attname IN ('from_account_id','to_account_id');

\echo === C1. current live view (correlated MAX)
EXPLAIN (ANALYZE, SUMMARY, TIMING OFF) SELECT account_id, balance, accrued FROM contract_ledger_live_positions;
\echo === C2. latest pointer table
EXPLAIN (ANALYZE, SUMMARY, TIMING OFF) SELECT account_id, balance, accrued FROM live_latest_ptr;
\echo === C3. business-day row (join)
EXPLAIN (ANALYZE, SUMMARY, TIMING OFF) SELECT account_id, balance, accrued FROM live_business_day;
\echo === C4. business-day row (functions)
EXPLAIN (ANALYZE, SUMMARY, TIMING OFF) SELECT account_id, balance, accrued FROM live_business_day_fn;
\echo === C5. latest view (grouped MAX)
EXPLAIN (ANALYZE, SUMMARY, TIMING OFF) SELECT account_id, balance, accrued FROM contract_ledger_latest_positions;
\echo === C6. one account, business-day (join) then (functions)
EXPLAIN (ANALYZE, SUMMARY, TIMING OFF) SELECT balance, accrued FROM live_business_day WHERE account_id = (SELECT id FROM accounts WHERE full_path = 'Liability:Savings:777');
EXPLAIN (ANALYZE, SUMMARY, TIMING OFF) SELECT balance, accrued FROM live_business_day_fn WHERE account_id = (SELECT id FROM accounts WHERE full_path = 'Liability:Savings:777');
\echo === agreement after the withdrawals
CREATE TEMP TABLE w_live AS SELECT account_id, balance, accrued FROM contract_ledger_live_positions;
SELECT 'ptr' AS candidate, count(*) FROM w_live v JOIN live_latest_ptr l ON l.account_id = v.account_id WHERE v.balance <> l.balance OR v.accrued <> l.accrued
UNION ALL SELECT 'bday', count(*) FROM w_live v JOIN live_business_day b ON b.account_id = v.account_id WHERE v.balance <> b.balance OR v.accrued <> b.accrued
UNION ALL SELECT 'bday_fn', count(*) FROM w_live v JOIN live_business_day_fn b ON b.account_id = v.account_id WHERE v.balance <> b.balance OR v.accrued <> b.accrued;
