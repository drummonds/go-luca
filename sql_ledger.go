package luca

import (
	"database/sql"
	"fmt"
)

// dbtx is the subset of database operations SQLLedger uses, satisfied by
// both *sql.DB and *sql.Tx so a ledger can run inside a caller's transaction.
type dbtx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// beginner is satisfied by *sql.DB but not *sql.Tx; see SQLLedger.begin.
type beginner interface {
	Begin() (*sql.Tx, error)
}

// SQLLedger is the SQL-backed Ledger implementation.
// Works with any database/sql driver (pglike, postgres, etc.).
type SQLLedger struct {
	db dbtx
}

// Compile-time interface check.
var _ Ledger = (*SQLLedger)(nil)

// WithTx returns a ledger view that runs every statement on tx. The caller
// owns the transaction: methods that normally manage their own transaction
// run directly on tx instead, nothing is committed by the ledger, and on
// error the caller should roll tx back.
func (l *SQLLedger) WithTx(tx *sql.Tx) *SQLLedger {
	return &SQLLedger{db: tx}
}

// begin starts a transaction when the ledger owns a *sql.DB. When the ledger
// is bound to a caller's transaction (WithTx), it returns that transaction
// with no-op commit and rollback — the caller owns the transaction's fate.
func (l *SQLLedger) begin() (q dbtx, commit func() error, rollback func() error, err error) {
	if b, ok := l.db.(beginner); ok {
		tx, err := b.Begin()
		if err != nil {
			return nil, nil, nil, err
		}
		return tx, tx.Commit, tx.Rollback, nil
	}
	noop := func() error { return nil }
	return l.db, noop, noop, nil
}

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
	if err := createSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return l, nil
}

// Close closes the underlying database connection. On a tx-bound ledger
// (WithTx) it is a no-op — the transaction's owner closes the database.
func (l *SQLLedger) Close() error {
	if c, ok := l.db.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}
