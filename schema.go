package luca

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	gdb "git.bytestone.uk/hum3/gobank-db"
	"github.com/google/uuid"

	_ "git.bytestone.uk/hum3/go-postgres"
)

// SchemaSQL is the DDL for the go-luca database schema.
// Exported so downstream projects and documentation tools (e.g. tbls)
// can inspect or recreate the schema.
const SchemaSQL = `
CREATE TABLE IF NOT EXISTS options (
    id TEXT PRIMARY KEY,
    key VARCHAR(200) NOT NULL UNIQUE,
    value VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS commodities (
    id TEXT PRIMARY KEY,
    code VARCHAR(50) NOT NULL UNIQUE,
    exponent INTEGER NOT NULL DEFAULT -2,
    datetime TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS commodity_metadata (
    id TEXT PRIMARY KEY,
    commodity_id TEXT NOT NULL REFERENCES commodities(id),
    key VARCHAR(200) NOT NULL,
    value VARCHAR(500) NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_commodity_metadata_unique ON commodity_metadata(commodity_id, key);

CREATE TABLE IF NOT EXISTS customers (
    id TEXT PRIMARY KEY,
    name VARCHAR(200) NOT NULL UNIQUE,
    max_balance_amount VARCHAR(50) NOT NULL DEFAULT '',
    max_balance_commodity VARCHAR(50) NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS customer_metadata (
    id TEXT PRIMARY KEY,
    customer_id TEXT NOT NULL REFERENCES customers(id),
    key VARCHAR(200) NOT NULL,
    value VARCHAR(500) NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_metadata_unique ON customer_metadata(customer_id, key);

CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    full_path VARCHAR(500) NOT NULL UNIQUE,
    account_type VARCHAR(50) NOT NULL,
    product VARCHAR(100) NOT NULL DEFAULT '',
    account_id VARCHAR(100) NOT NULL DEFAULT '',
    address VARCHAR(100) NOT NULL DEFAULT '',
    is_pending BOOLEAN DEFAULT FALSE,
    commodity VARCHAR(50) NOT NULL DEFAULT 'GBP' REFERENCES commodities(code),
    customer_id TEXT REFERENCES customers(id),
    gross_interest_rate NUMERIC(10,6) NOT NULL DEFAULT 0,
    interest_method VARCHAR(20) NOT NULL DEFAULT '',
    interest_accumulator BIGINT NOT NULL DEFAULT 0,
    opened_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS movements (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL,
    from_account_id TEXT NOT NULL REFERENCES accounts(id),
    to_account_id TEXT NOT NULL REFERENCES accounts(id),
    amount BIGINT NOT NULL,
    code VARCHAR(14) NOT NULL,
    ledger INTEGER NOT NULL DEFAULT 0,
    pending_id BIGINT NOT NULL DEFAULT 0,
    user_data_64 BIGINT NOT NULL DEFAULT 0,
    value_time TIMESTAMP NOT NULL,
    knowledge_time TIMESTAMP DEFAULT NOW(),
    description VARCHAR(500) NOT NULL DEFAULT '',
    period_anchor VARCHAR(1) NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_movements_from ON movements(from_account_id, value_time);
CREATE INDEX IF NOT EXISTS idx_movements_to ON movements(to_account_id, value_time);
CREATE INDEX IF NOT EXISTS idx_movements_batch ON movements(batch_id);
CREATE INDEX IF NOT EXISTS idx_movements_code ON movements(to_account_id, code, value_time);
-- A day's movements across every account (a general ledger's journal
-- derives from them): bounded by the day, not the account.
CREATE INDEX IF NOT EXISTS idx_movements_value_time ON movements(value_time);

CREATE TABLE IF NOT EXISTS balances_live (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    balance_date TIMESTAMP NOT NULL,
    balance BIGINT NOT NULL,
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_balances_live_unique
    ON balances_live(account_id, balance_date);
-- A day's positions across every account (reconciliation, the pass's
-- unprojected list, a business-day live read): the day first, then the
-- account so a (day, account) probe is one lookup.
CREATE INDEX IF NOT EXISTS idx_balances_live_day
    ON balances_live(balance_date, account_id);

-- The business day: the day the pass is projecting, and the business day
-- it advanced from (NULL for the first). One row, moved by AdvanceDay.
-- While the pass runs, every account's latest position is on one of the
-- two, which is what makes the live view a read of two day slices.
CREATE TABLE IF NOT EXISTS ledger_day (
    day TIMESTAMP NOT NULL,
    prev_day TIMESTAMP
);

CREATE TABLE IF NOT EXISTS aliases (
    id TEXT PRIMARY KEY,
    name VARCHAR(200) NOT NULL UNIQUE,
    account_path VARCHAR(500) NOT NULL REFERENCES accounts(full_path),
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS data_points (
    id TEXT PRIMARY KEY,
    value_time TIMESTAMP NOT NULL,
    knowledge_time TIMESTAMP DEFAULT NOW(),
    param_name VARCHAR(200) NOT NULL,
    param_type VARCHAR(20) NOT NULL DEFAULT 'string',
    param_value VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_data_points_name_time ON data_points(param_name, value_time);

CREATE TABLE IF NOT EXISTS movement_metadata (
    id TEXT PRIMARY KEY,
    batch_id TEXT NOT NULL,
    key VARCHAR(200) NOT NULL,
    value VARCHAR(500) NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_movement_metadata_unique ON movement_metadata(batch_id, key);

-- Expand-only additions for positions (see positions.go). A commodity's
-- unit is its minor units per major unit (100 for GBP), so a view can turn
-- stored integers into NUMERIC money without a power function.
ALTER TABLE commodities ADD COLUMN IF NOT EXISTS unit BIGINT NOT NULL DEFAULT 0;
UPDATE commodities SET unit = CASE exponent
    WHEN 0 THEN 1 WHEN -1 THEN 10 WHEN -2 THEN 100 WHEN -3 THEN 1000 WHEN -4 THEN 10000
    WHEN -5 THEN 100000 WHEN -6 THEN 1000000 WHEN -7 THEN 10000000 WHEN -8 THEN 100000000
    ELSE 1 END WHERE unit = 0;
ALTER TABLE balances_live ADD COLUMN IF NOT EXISTS accrued_num BIGINT NOT NULL DEFAULT 0;
ALTER TABLE balances_live ADD COLUMN IF NOT EXISTS accrued_den BIGINT NOT NULL DEFAULT 1;
-- next_day is the midnight after balance_date, stored so the live view can
-- find the movements a position does not yet include without date
-- arithmetic (which pglike cannot yet translate on a qualified column).
ALTER TABLE balances_live ADD COLUMN IF NOT EXISTS next_day TIMESTAMP;
UPDATE balances_live SET next_day = balance_date + INTERVAL '1 day' WHERE next_day IS NULL;

-- Contract views (gobank ADR-0001): what other components may read. Money
-- crosses as NUMERIC in major units and how it is stored stays in here.
-- (No semicolons in these comments: the schema runner splits on them.)
DROP VIEW IF EXISTS contract_ledger_movements;
CREATE VIEW contract_ledger_movements AS
    SELECT m.id, m.from_account_id, m.to_account_id, fa.full_path AS from_path, ta.full_path AS to_path,
           m.amount, m.code, m.value_time, m.knowledge_time, m.description
    FROM movements m
    JOIN accounts fa ON fa.id = m.from_account_id
    JOIN accounts ta ON ta.id = m.to_account_id;

-- The position views read one another, so every one is dropped first and
-- they are created bottom-up: PostgreSQL will not drop a view another
-- depends on.
DROP VIEW IF EXISTS contract_ledger_live_positions;
DROP VIEW IF EXISTS contract_ledger_latest_positions;
DROP VIEW IF EXISTS contract_ledger_eod_positions;
DROP VIEW IF EXISTS ledger_latest_projections;

-- End-of-day positions: one row per account per projected day, from the
-- stored projections. The cheap view.
CREATE VIEW contract_ledger_eod_positions AS
    SELECT p.account_id, a.full_path, a.commodity, p.balance_date AS day,
           round(p.balance::numeric / c.unit, -c.exponent) AS balance,
           round(p.accrued_num::numeric / p.accrued_den / c.unit, 7) AS accrued
    FROM balances_live p
    JOIN accounts a ON a.id = p.account_id
    JOIN commodities c ON c.code = a.commodity;

-- Each account's latest projection, in minor units, found with one grouped
-- MAX rather than a correlated subquery per account. Internal: the latest
-- view reads it. (Not DISTINCT ON or LATERAL: this runs on pglike.)
CREATE VIEW ledger_latest_projections AS
    SELECT p.account_id, p.balance_date, p.next_day, p.balance, p.accrued_num, p.accrued_den
    FROM balances_live p
    JOIN (SELECT account_id, MAX(balance_date) AS latest
          FROM balances_live GROUP BY account_id) q
      ON q.account_id = p.account_id AND q.latest = p.balance_date;

-- Latest positions: the end-of-day row every consumer wanting "the latest
-- day per account" used to find with its own correlated MAX(day).
CREATE VIEW contract_ledger_latest_positions AS
    SELECT p.account_id, a.full_path, a.commodity, p.balance_date AS day,
           round(p.balance::numeric / c.unit, -c.exponent) AS balance,
           round(p.accrued_num::numeric / p.accrued_den / c.unit, 7) AS accrued
    FROM ledger_latest_projections p
    JOIN accounts a ON a.id = p.account_id
    JOIN commodities c ON c.code = a.commodity;

-- Live positions: every account's position on the business day, else on
-- the previous business day (the pass has not reached it), else its
-- latest row (stale: the pass has skipped it or never advanced), plus
-- every movement valued on or after that row's next_day. The two days
-- are read as two slices of the day index, so a whole-ledger read costs
-- the accounts, not the positions, and the one-account read is a probe
-- on either driver (issue #7, docs/queries.md). The day is an
-- uncorrelated scalar subquery, evaluated once: PostgreSQL plans it as
-- the constant it is, where a join to the row made it hash the whole
-- table, and pglike has no functions. Mid-pass, an account already
-- projected to day D is projection(D) plus movements from D+1, one
-- still on D-1 is projection(D-1) plus movements from D, and both are
-- that account's exact live balance. Only accrued mixes: it is read off
-- the position found, so during a pass the sum is part day D and part
-- day D-1, which is the bank's actual accrual state until the pass
-- completes.
CREATE VIEW contract_ledger_live_positions AS
    SELECT b.account_id, b.full_path, b.commodity,
           round(b.balance_minor::numeric / b.unit, -b.exponent) AS balance,
           round(b.accrued_num::numeric / b.accrued_den / b.unit, 7) AS accrued
    FROM (
        SELECT l.account_id, l.full_path, l.commodity, l.unit, l.exponent,
               l.accrued_num, l.accrued_den,
               l.balance
               + COALESCE((SELECT SUM(m.amount) FROM movements m
                           WHERE m.to_account_id = l.account_id AND m.value_time >= l.next_day), 0)
               - COALESCE((SELECT SUM(m.amount) FROM movements m
                           WHERE m.from_account_id = l.account_id AND m.value_time >= l.next_day), 0)
                 AS balance_minor
        FROM (
            SELECT a.id AS account_id, a.full_path, a.commodity, c.unit, c.exponent,
                   COALESCE(p.balance, y.balance,
                            (SELECT z.balance FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1),
                            0) AS balance,
                   COALESCE(p.next_day, y.next_day,
                            (SELECT z.next_day FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1),
                            '0001-01-01') AS next_day,
                   COALESCE(p.accrued_num, y.accrued_num,
                            (SELECT z.accrued_num FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1),
                            0) AS accrued_num,
                   COALESCE(p.accrued_den, y.accrued_den,
                            (SELECT z.accrued_den FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1),
                            1) AS accrued_den
            FROM accounts a
            JOIN commodities c ON c.code = a.commodity
            LEFT JOIN balances_live p ON p.account_id = a.id AND p.balance_date = (SELECT day FROM ledger_day)
            LEFT JOIN balances_live y ON y.account_id = a.id AND y.balance_date = (SELECT prev_day FROM ledger_day)
        ) l
    ) b;
`

// createSchema executes the DDL statements to create tables and indexes.
func createSchema(db *sql.DB, schema string) error {
	return gdb.Migrate(context.Background(), db, schema)
}

// CreateSchemaDB creates a pglike (SQLite) database at path with the go-luca
// schema and sample data suitable for documentation tools like tbls.
// The caller is responsible for closing the returned *sql.DB.
func CreateSchemaDB(path string) (*sql.DB, error) {
	db, err := sql.Open("pglike", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := CreateSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// CreateSchema applies the go-luca schema to an empty database and fills it
// with the sample data. It works on any database/sql connection the schema
// runs on, pglike or PostgreSQL, so documentation can be generated from
// either.
func CreateSchema(db *sql.DB) error {
	if err := gdb.Migrate(context.Background(), db, SchemaSQL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if err := insertSampleData(db); err != nil {
		return fmt.Errorf("insert sample data: %w", err)
	}
	return nil
}

// insertSampleData populates the schema with representative sample data
// so documentation tools can show realistic column values and relationships.
func insertSampleData(db *sql.DB) error {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Sample commodity (must exist before accounts due to FK)
	_, err := db.Exec(
		`INSERT INTO commodities (id, code, exponent, unit) VALUES ($1, $2, $3, $4)`,
		uuid.New().String(), "GBP", -2, commodityUnit(-2),
	)
	if err != nil {
		return fmt.Errorf("insert commodity GBP: %w", err)
	}

	// Sample accounts covering all five types
	accounts := []struct {
		id             string
		fullPath       string
		accountType    string
		product        string
		accountID      string
		address        string
		isPending      bool
		commodity      string
		interestRate   float64
		interestMethod string
	}{
		{uuid.New().String(), "Asset:Bank:Current:Main", "Asset", "Bank", "Current", "Main", false, "GBP", 0, ""},
		{uuid.New().String(), "Asset:Bank:Savings:Main", "Asset", "Bank", "Savings", "Main", false, "GBP", 0.0425, "simple_daily"},
		{uuid.New().String(), "Liability:Mortgage:Home:Main", "Liability", "Mortgage", "Home", "Main", false, "GBP", 0.045, "simple_daily"},
		{uuid.New().String(), "Equity:OpeningBalances", "Equity", "OpeningBalances", "", "", false, "GBP", 0, ""},
		{uuid.New().String(), "Income:Salary", "Income", "Salary", "", "", false, "GBP", 0, ""},
		{uuid.New().String(), "Expense:Groceries", "Expense", "Groceries", "", "", false, "GBP", 0, ""},
		{uuid.New().String(), "Expense:Interest", "Expense", "Interest", "", "", false, "GBP", 0, ""},
	}

	for _, a := range accounts {
		_, err := db.Exec(
			`INSERT INTO accounts (id, full_path, account_type, product, account_id, address, is_pending, commodity, gross_interest_rate, interest_method, opened_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			a.id, a.fullPath, a.accountType, a.product, a.accountID, a.address, a.isPending, a.commodity, a.interestRate, a.interestMethod, nil,
		)
		if err != nil {
			return fmt.Errorf("insert account %s: %w", a.fullPath, err)
		}
	}

	// Sample movements showing different patterns
	linkedBatch := uuid.New().String() // shared by linked movements

	type sampleMovement struct {
		id          string
		batchID     string
		fromID      string
		toID        string
		amount      Amount
		code        string
		ledger      int32
		pendingID   int64
		userData64  int64
		valueTime   time.Time
		description string
	}
	movements := []sampleMovement{
		{uuid.New().String(), uuid.New().String(), accounts[3].id, accounts[0].id, 250000, CodeOpeningBalance, 0, 0, 0, today, "Opening balance"},
		{uuid.New().String(), uuid.New().String(), accounts[4].id, accounts[0].id, 350000, CodeCreditReceived, 0, 0, 0, today, "March salary"},
		{uuid.New().String(), linkedBatch, accounts[0].id, accounts[5].id, 4523, CodeCreditIssued, 0, 0, 0, today, "Weekly shop"},
		{uuid.New().String(), linkedBatch, accounts[0].id, accounts[5].id, 1299, CodeCreditIssued, 0, 0, 0, today, "Coffee and snacks"},
		{uuid.New().String(), uuid.New().String(), accounts[0].id, accounts[1].id, 100000, CodeBookTransfer, 0, 0, 0, today, "Transfer to savings"},
		{uuid.New().String(), uuid.New().String(), accounts[6].id, accounts[1].id, 12, CodeInterestAccrual, 0, 0, 0, today, "Daily interest for savings"},
	}

	for _, m := range movements {
		_, err := db.Exec(
			`INSERT INTO movements (id, batch_id, from_account_id, to_account_id, amount, code, ledger, pending_id, user_data_64, value_time, description)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			m.id, m.batchID, m.fromID, m.toID, m.amount, m.code, m.ledger, m.pendingID, m.userData64, m.valueTime, m.description,
		)
		if err != nil {
			return fmt.Errorf("insert movement: %w", err)
		}
	}

	// Sample live balance
	_, err = db.Exec(
		`INSERT INTO balances_live (id, account_id, balance_date, balance) VALUES ($1, $2, $3, $4)`,
		uuid.New().String(), accounts[1].id, today, 100012,
	)
	if err != nil {
		return fmt.Errorf("insert live balance: %w", err)
	}

	return nil
}
