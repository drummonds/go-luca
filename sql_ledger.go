package luca

import (
	"database/sql"
	"fmt"
)

// SQLLedger is the SQL-backed Ledger implementation.
// Works with any database/sql driver (pglike, postgres, etc.).
type SQLLedger struct {
	db *sql.DB

	// InterestFunc is called to compute daily interest. If nil, the built-in
	// default (balance * rate / 365) is used. Products set this to implement
	// methods like discrete_daily, actual_actual, etc.
	InterestFunc InterestFunc
}

// Compile-time interface check.
var _ Ledger = (*SQLLedger)(nil)

// NewLedger opens a pglike database and ensures the schema exists.
// dsn can be ":memory:" for tests or a file path for persistence.
// Returns the Ledger interface.
func NewLedger(dsn string) (*SQLLedger, error) {
	db, err := sql.Open("pglike", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return NewSQLLedger(db)
}

// NewSQLLedger wraps a pre-opened *sql.DB and ensures the schema exists.
// Use this to connect with any database/sql driver (e.g. real postgres).
func NewSQLLedger(db *sql.DB) (*SQLLedger, error) {
	l := &SQLLedger{db: db}
	if err := l.createSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return l, nil
}

// Close closes the underlying database connection.
func (l *SQLLedger) Close() error {
	return l.db.Close()
}
