package luca

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Movement code constants (TigerBeetle-inspired).
const (
	CodeNormal          int16 = 0
	CodeInterestAccrual int16 = 1
)

// LiveBalance is a pre-computed end-of-day balance snapshot stored in balances_live.
type LiveBalance struct {
	AccountID   int64
	BalanceDate time.Time
	Balance     int64
}

// AccountType represents one of the five fundamental account categories.
type AccountType string

const (
	AccountTypeAsset     AccountType = "Asset"
	AccountTypeLiability AccountType = "Liability"
	AccountTypeEquity    AccountType = "Equity"
	AccountTypeIncome    AccountType = "Income"
	AccountTypeExpense   AccountType = "Expense"
)

var validAccountTypes = map[AccountType]bool{
	AccountTypeAsset:     true,
	AccountTypeLiability: true,
	AccountTypeEquity:    true,
	AccountTypeIncome:    true,
	AccountTypeExpense:   true,
}

// Account represents a node in the chart of accounts.
// The FullPath follows the hierarchical format: Type:Product:AccountID:Address
type Account struct {
	ID                 int64
	FullPath           string
	Type               AccountType
	Product            string
	AccountID          string
	Address            string
	IsPending          bool
	Currency           string
	Exponent           int // e.g. -2 for GBP pence, -5 for high precision
	AnnualInterestRate float64
	CreatedAt          time.Time
}

// Movement represents a directed flow of value from one account to another.
// Amount is an integer in the smallest currency unit, stored at the
// higher-precision exponent of the two accounts involved.
type Movement struct {
	ID            int64
	BatchID       int64
	FromAccountID int64
	ToAccountID   int64
	Amount        int64
	Code          int16 // category/reason enum (TB-inspired)
	Ledger        int32 // partition identifier (TB-inspired)
	PendingID     int64 // two-phase post/void; 0 = N/A (TB-inspired)
	UserData64    int64 // external reference (TB-inspired)
	ValueTime     time.Time
	KnowledgeTime time.Time
	Description   string
}

// MovementInput describes a movement to be recorded (before it gets a DB id/batch).
type MovementInput struct {
	FromAccountID int64
	ToAccountID   int64
	Amount        int64
	Code          int16
	Ledger        int32
	PendingID     int64
	UserData64    int64
	Description   string
}

// Ledger is the top-level object that manages accounts, movements,
// and the database connection.
type Ledger struct {
	db *sql.DB
}

// NewLedger opens a database connection and ensures the schema exists.
// dsn can be ":memory:" for tests or a file path for persistence.
func NewLedger(dsn string) (*Ledger, error) {
	db, err := sql.Open("pglike", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	l := &Ledger{db: db}
	if err := l.createSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return l, nil
}

// Close closes the underlying database connection.
func (l *Ledger) Close() error {
	return l.db.Close()
}

// parseFullPath splits a hierarchical account path into its components.
// Paths follow the format: Type:Product:AccountID:Address
// Minimum is Type:Product (two components).
// If the address is "Pending", isPending is set to true.
func parseFullPath(fullPath string) (accountType AccountType, product, accountID, address string, isPending bool, err error) {
	parts := strings.Split(fullPath, ":")
	if len(parts) < 2 {
		return "", "", "", "", false, fmt.Errorf("path must have at least Type:Product, got %q", fullPath)
	}

	accountType = AccountType(parts[0])
	if !validAccountTypes[accountType] {
		return "", "", "", "", false, fmt.Errorf("invalid account type %q", parts[0])
	}

	product = parts[1]
	if len(parts) >= 3 {
		accountID = parts[2]
	}
	if len(parts) >= 4 {
		address = parts[3]
		isPending = address == "Pending"
	}

	return accountType, product, accountID, address, isPending, nil
}
