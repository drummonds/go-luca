\timing on
SET statement_timeout = '300s';

-- Candidate 1: latest pointer kept on write (one row per account).
DROP TABLE IF EXISTS balances_latest CASCADE;
CREATE TABLE balances_latest AS
  SELECT account_id, MAX(balance_date) AS balance_date FROM balances_live GROUP BY account_id;
ALTER TABLE balances_latest ADD PRIMARY KEY (account_id);
ANALYZE balances_latest;
CREATE OR REPLACE VIEW live_latest_ptr AS
    SELECT a.id AS account_id, a.full_path, a.commodity,
           round((COALESCE(p.balance, 0)
               + COALESCE((SELECT SUM(m.amount) FROM movements m
                           WHERE m.to_account_id = a.id AND m.value_time >= COALESCE(p.next_day, '0001-01-01')), 0)
               - COALESCE((SELECT SUM(m.amount) FROM movements m
                           WHERE m.from_account_id = a.id AND m.value_time >= COALESCE(p.next_day, '0001-01-01')), 0)
               )::numeric / c.unit, -c.exponent) AS balance,
           round(COALESCE(p.accrued_num, 0)::numeric / COALESCE(p.accrued_den, 1) / c.unit, 7) AS accrued
    FROM accounts a
    JOIN commodities c ON c.code = a.commodity
    LEFT JOIN balances_latest bl ON bl.account_id = a.id
    LEFT JOIN balances_live p ON p.account_id = bl.account_id AND p.balance_date = bl.balance_date;

-- Candidate 2: business-day row (today's, else yesterday's, else latest).
DROP TABLE IF EXISTS ledger_day CASCADE;
CREATE TABLE ledger_day AS SELECT '2025-07-19'::timestamp AS day, '2025-07-18'::timestamp AS prev_day;
CREATE OR REPLACE VIEW live_business_day AS
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
        CROSS JOIN ledger_day d
        LEFT JOIN balances_live p ON p.account_id = a.id AND p.balance_date = d.day
        LEFT JOIN balances_live y ON y.account_id = a.id AND y.balance_date = d.prev_day
    ) l;

\echo === sizes
SELECT relname, pg_size_pretty(pg_relation_size(oid)) FROM pg_class WHERE relname IN ('balances_live','movements','accounts','idx_balances_live_unique','idx_balances_live_day','idx_movements_to','idx_movements_from','balances_live_pkey','movements_pkey') ORDER BY 1;

\echo === candidate: business-day row, whole ledger
EXPLAIN (ANALYZE, SUMMARY) SELECT account_id, balance, accrued FROM live_business_day;
\echo === candidate: latest pointer table, whole ledger
EXPLAIN (ANALYZE, SUMMARY) SELECT account_id, balance, accrued FROM live_latest_ptr;
\echo === latest view (grouped MAX), whole ledger
EXPLAIN (ANALYZE, SUMMARY) SELECT account_id, balance, accrued FROM contract_ledger_latest_positions;
\echo === current live view (correlated MAX), whole ledger
EXPLAIN (ANALYZE, SUMMARY) SELECT account_id, balance, accrued FROM contract_ledger_live_positions;

\echo === one account, each
EXPLAIN (ANALYZE, SUMMARY) SELECT balance, accrued FROM contract_ledger_live_positions WHERE account_id = (SELECT id FROM accounts WHERE full_path = 'Liability:Savings:777');
EXPLAIN (ANALYZE, SUMMARY) SELECT balance, accrued FROM live_latest_ptr WHERE account_id = (SELECT id FROM accounts WHERE full_path = 'Liability:Savings:777');
EXPLAIN (ANALYZE, SUMMARY) SELECT balance, accrued FROM live_business_day WHERE account_id = (SELECT id FROM accounts WHERE full_path = 'Liability:Savings:777');

\echo === agreement, pairwise against the shipped view (materialised first)
CREATE TEMP TABLE v_live AS SELECT account_id, balance, accrued FROM contract_ledger_live_positions;
CREATE TEMP TABLE v_ptr AS SELECT account_id, balance, accrued FROM live_latest_ptr;
CREATE TEMP TABLE v_bday AS SELECT account_id, balance, accrued FROM live_business_day;
SELECT 'ptr' AS candidate, count(*) AS disagreements FROM v_live v JOIN v_ptr l ON l.account_id = v.account_id WHERE v.balance <> l.balance OR v.accrued <> l.accrued
UNION ALL
SELECT 'bday', count(*) FROM v_live v JOIN v_bday b ON b.account_id = v.account_id WHERE v.balance <> b.balance OR v.accrued <> b.accrued;
SELECT (SELECT count(*) FROM v_live) live_rows, (SELECT count(*) FROM v_ptr) ptr_rows, (SELECT count(*) FROM v_bday) bday_rows;
