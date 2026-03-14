package luca

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/drummonds/go-postgres"
)

// SchemaSQL is the DDL for the go-luca database schema.
// Exported so downstream projects and documentation tools (e.g. tbls)
// can inspect or recreate the schema.
const SchemaSQL = `
CREATE TABLE IF NOT EXISTS accounts (
    id SERIAL PRIMARY KEY,
    full_path VARCHAR(500) NOT NULL UNIQUE,
    account_type VARCHAR(50) NOT NULL,
    product VARCHAR(100) NOT NULL DEFAULT '',
    account_id VARCHAR(100) NOT NULL DEFAULT '',
    address VARCHAR(100) NOT NULL DEFAULT '',
    is_pending BOOLEAN DEFAULT FALSE,
    currency VARCHAR(10) NOT NULL DEFAULT 'GBP',
    exponent INTEGER NOT NULL DEFAULT -2,
    annual_interest_rate NUMERIC(10,6) NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS movements (
    id SERIAL PRIMARY KEY,
    batch_id INTEGER NOT NULL,
    from_account_id INTEGER NOT NULL,
    to_account_id INTEGER NOT NULL,
    amount BIGINT NOT NULL,
    code SMALLINT NOT NULL DEFAULT 0,
    ledger INTEGER NOT NULL DEFAULT 0,
    pending_id BIGINT NOT NULL DEFAULT 0,
    user_data_64 BIGINT NOT NULL DEFAULT 0,
    value_time TIMESTAMP NOT NULL,
    knowledge_time TIMESTAMP DEFAULT NOW(),
    description VARCHAR(500) NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_movements_from ON movements(from_account_id, value_time);
CREATE INDEX IF NOT EXISTS idx_movements_to ON movements(to_account_id, value_time);
CREATE INDEX IF NOT EXISTS idx_movements_batch ON movements(batch_id);
CREATE INDEX IF NOT EXISTS idx_movements_code ON movements(to_account_id, code, value_time);

CREATE TABLE IF NOT EXISTS balances_live (
    id SERIAL PRIMARY KEY,
    account_id INTEGER NOT NULL,
    balance_date TIMESTAMP NOT NULL,
    balance BIGINT NOT NULL,
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_balances_live_unique
    ON balances_live(account_id, balance_date);
`

// createSchema executes the DDL statements to create tables and indexes.
func (l *SQLLedger) createSchema() error {
	_, err := l.db.Exec(SchemaSQL)
	return err
}

// CreateSchemaDB creates a pglike (SQLite) database at path with the go-luca
// schema and sample data suitable for documentation tools like tbls.
// The caller is responsible for closing the returned *sql.DB.
func CreateSchemaDB(path string) (*sql.DB, error) {
	db, err := sql.Open("pglike", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if _, err := db.Exec(SchemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if err := insertSampleData(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("insert sample data: %w", err)
	}
	return db, nil
}

// insertSampleData populates the schema with representative sample data
// so documentation tools can show realistic column values and relationships.
func insertSampleData(db *sql.DB) error {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Sample accounts covering all five types
	accounts := []struct {
		fullPath     string
		accountType  string
		product      string
		accountID    string
		address      string
		isPending    bool
		currency     string
		exponent     int
		interestRate float64
	}{
		{"Asset:Bank:Current:Main", "Asset", "Bank", "Current", "Main", false, "GBP", -2, 0},
		{"Asset:Bank:Savings:Main", "Asset", "Bank", "Savings", "Main", false, "GBP", -2, 0.0425},
		{"Liability:Mortgage:Home:Main", "Liability", "Mortgage", "Home", "Main", false, "GBP", -2, 0.045},
		{"Equity:OpeningBalances", "Equity", "OpeningBalances", "", "", false, "GBP", -2, 0},
		{"Income:Salary", "Income", "Salary", "", "", false, "GBP", -2, 0},
		{"Expense:Groceries", "Expense", "Groceries", "", "", false, "GBP", -2, 0},
		{"Expense:Interest", "Expense", "Interest", "", "", false, "GBP", -2, 0},
	}

	for _, a := range accounts {
		_, err := db.Exec(
			`INSERT INTO accounts (full_path, account_type, product, account_id, address, is_pending, currency, exponent, annual_interest_rate)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			a.fullPath, a.accountType, a.product, a.accountID, a.address, a.isPending, a.currency, a.exponent, a.interestRate,
		)
		if err != nil {
			return fmt.Errorf("insert account %s: %w", a.fullPath, err)
		}
	}

	// Sample movements showing different patterns
	movements := []struct {
		batchID     int
		fromID      int
		toID        int
		amount      Amount
		code        int16
		ledger      int32
		pendingID   int64
		userData64  int64
		valueTime   time.Time
		description string
	}{
		{1, 4, 1, 250000, 0, 0, 0, 0, today, "Opening balance"},
		{2, 5, 1, 350000, 0, 0, 0, 0, today, "March salary"},
		{3, 1, 6, 4523, 0, 0, 0, 0, today, "Weekly shop"},
		{3, 1, 6, 1299, 0, 0, 0, 0, today, "Coffee and snacks"},
		{4, 1, 2, 100000, 0, 0, 0, 0, today, "Transfer to savings"},
		{5, 7, 2, 12, 1, 0, 0, 0, today, "Daily interest for savings"},
	}

	for _, m := range movements {
		_, err := db.Exec(
			`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, code, ledger, pending_id, user_data_64, value_time, description)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			m.batchID, m.fromID, m.toID, m.amount, m.code, m.ledger, m.pendingID, m.userData64, m.valueTime, m.description,
		)
		if err != nil {
			return fmt.Errorf("insert movement: %w", err)
		}
	}

	// Sample live balance
	_, err := db.Exec(
		`INSERT INTO balances_live (account_id, balance_date, balance) VALUES ($1, $2, $3)`,
		2, today, 100012,
	)
	if err != nil {
		return fmt.Errorf("insert live balance: %w", err)
	}

	return nil
}
