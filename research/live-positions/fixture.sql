\timing on
\set ON_ERROR_STOP on
\set accounts 20000
\set block 1000
DROP SCHEMA public CASCADE; CREATE SCHEMA public;
\i /tmp/schema.sql
INSERT INTO commodities (id, code, exponent, unit) VALUES (gen_random_uuid()::text, 'GBP', -2, 100);
INSERT INTO accounts (id, full_path, account_type, product, account_id, commodity)
  VALUES (gen_random_uuid()::text, 'Equity:Capital', 'Equity', '', '', 'GBP');
INSERT INTO accounts (id, full_path, account_type, product, account_id, commodity)
  SELECT gen_random_uuid()::text, 'Liability:Savings:' || n, 'Liability', 'Savings', n::text, 'GBP'
  FROM generate_series(1, :accounts) n;
-- Load without the secondary indexes; rebuild them once at the end.
DROP INDEX idx_movements_from, idx_movements_to, idx_movements_batch, idx_movements_code, idx_movements_value_time;
DROP INDEX idx_balances_live_unique, idx_balances_live_day;
-- opening movement per account on day 1
INSERT INTO movements (id, batch_id, from_account_id, to_account_id, amount, code, value_time, description)
  SELECT gen_random_uuid()::text, gen_random_uuid()::text, e.id, a.id, 100000, 'OPEN', '2025-01-01 09:00', 'open'
  FROM accounts a, (SELECT id FROM accounts WHERE full_path = 'Equity:Capital') e
  WHERE a.account_type = 'Liability';
-- 200 days of positions, in blocks of :block accounts, one statement each;
-- odd accounts still on day 199 (mid-pass)
SELECT format($f$INSERT INTO balances_live (id, account_id, balance_date, next_day, balance, accrued_num, accrued_den)
  SELECT gen_random_uuid()::text, a.id, d, d + interval '1 day', 100000, 1, 365
  FROM accounts a, generate_series('2025-01-01'::timestamp, '2025-07-19'::timestamp, '1 day') d
  WHERE a.account_type = 'Liability' AND a.account_id::int BETWEEN %s AND %s
    AND NOT (d = '2025-07-19'::timestamp AND a.account_id::int %% 2 = 1)$f$, lo, lo + :block - 1)
  FROM generate_series(1, :accounts, :block) lo \gexec
-- a quarter of accounts moved after their latest projection
INSERT INTO movements (id, batch_id, from_account_id, to_account_id, amount, code, value_time, description)
  SELECT gen_random_uuid()::text, gen_random_uuid()::text, e.id, a.id, 250, 'DEP', '2025-07-20 10:00', 'deposit'
  FROM accounts a, (SELECT id FROM accounts WHERE full_path = 'Equity:Capital') e
  WHERE a.account_type = 'Liability' AND a.account_id::int % 4 = 0;
\echo === rebuilding indexes
CREATE INDEX idx_movements_from ON movements(from_account_id, value_time);
CREATE INDEX idx_movements_to ON movements(to_account_id, value_time);
CREATE INDEX idx_movements_batch ON movements(batch_id);
CREATE INDEX idx_movements_code ON movements(to_account_id, code, value_time);
CREATE INDEX idx_movements_value_time ON movements(value_time);
CREATE UNIQUE INDEX idx_balances_live_unique ON balances_live(account_id, balance_date);
CREATE INDEX idx_balances_live_day ON balances_live(balance_date, account_id);
VACUUM ANALYZE;
SELECT (SELECT count(*) FROM accounts) accounts, (SELECT count(*) FROM balances_live) positions, (SELECT count(*) FROM movements) movements,
       pg_size_pretty(pg_total_relation_size('balances_live')) positions_size;
