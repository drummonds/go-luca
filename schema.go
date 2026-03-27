package luca

import (
	"database/sql"
	"fmt"
	"time"

	gdb "codeberg.org/hum3/gobank-db"
	"github.com/google/uuid"

	_ "codeberg.org/hum3/go-postgres"
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

CREATE TABLE IF NOT EXISTS balances_live (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id),
    balance_date TIMESTAMP NOT NULL,
    balance BIGINT NOT NULL,
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_balances_live_unique
    ON balances_live(account_id, balance_date);

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
`

// createSchema executes the DDL statements to create tables and indexes.
func (l *SQLLedger) createSchema() error {
	return gdb.ExecStatements(l.db, SchemaSQL)
}

// CreateSchemaDB creates a pglike (SQLite) database at path with the go-luca
// schema and sample data suitable for documentation tools like tbls.
// The caller is responsible for closing the returned *sql.DB.
func CreateSchemaDB(path string) (*sql.DB, error) {
	db, err := sql.Open("pglike", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := gdb.ExecStatements(db, SchemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if err := insertSampleData(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("insert sample data: %w", err)
	}
	return db, nil
}

// insertSampleData populates the schema with representative sample data
// so documentation tools can show realistic column values and relationships.
func insertSampleData(db *sql.DB) error {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Sample commodity (must exist before accounts due to FK)
	_, err := db.Exec(
		`INSERT INTO commodities (id, code, exponent) VALUES ($1, $2, $3)`,
		uuid.New().String(), "GBP", -2,
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
