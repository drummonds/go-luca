package luca

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// ErrNotImplemented is returned by Ledger backends that don't support a method.
var ErrNotImplemented = errors.New("not implemented")

// Amount represents a monetary value in the smallest currency unit (e.g. pence).
// Currently backed by int64; designed for future migration to 128-bit.
type Amount int64

// Movement code constants — ISO 20022 BTC mnemonics in DOMAIN:FAMILY:SUBFAMILY format.
const (
	CodeBookTransfer    = "PMNT:RCDT:BOOK" // internal book transfer
	CodeInterestAccrual = "LDAS:FTDP:INTR" // deposit interest accrual
	CodeCreditReceived  = "PMNT:RCDT:DMCT" // received domestic credit transfer
	CodeCreditIssued    = "PMNT:ICDT:DMCT" // issued domestic credit transfer
	CodeFee             = "ACMT:MDOP:FEES" // fee charge
	CodeOpeningBalance  = "ACMT:MCOP:OTHR" // opening balance / adjustment
)

// Ledger is the interface for all ledger backends.
type Ledger interface {
	Close() error

	// Accounts
	CreateAccount(fullPath string, currency string, exponent int, annualInterestRate float64) (*Account, error)
	GetAccount(fullPath string) (*Account, error)
	GetAccountByID(id string) (*Account, error)
	ListAccounts(typeFilter AccountType) ([]*Account, error)

	// Movements
	RecordMovement(fromAccountID, toAccountID string, amount Amount, code string, valueTime time.Time, description string) (*Movement, error)
	RecordLinkedMovements(movements []MovementInput, valueTime time.Time) (string, error)
	RecordMovementWithProjections(fromAccountID, toAccountID string, amount Amount, code string, valueTime time.Time, description string) (*Movement, error)

	// Balances
	Balance(accountID string) (Amount, error)
	BalanceAt(accountID string, at time.Time) (Amount, error)
	BalanceByPath(pathPrefix string, at time.Time) (Amount, int, error)
	DailyBalances(accountID string, from, to time.Time) ([]DailyBalance, error)
	GetLiveBalance(accountID string, date time.Time) (*LiveBalance, error)

	// Interest
	EnsureInterestAccounts() error
	CalculateDailyInterest(accountID string, date time.Time) (*InterestResult, error)
	RunDailyInterest(date time.Time) ([]InterestResult, error)
	RunInterestForPeriod(from, to time.Time) ([]InterestResult, error)

	// Import/Export
	ListMovements() ([]MovementWithPaths, error)
	Export(w io.Writer) error
	Import(r io.Reader, opts *ImportOptions) error
	ImportString(s string, opts *ImportOptions) error
}

// LiveBalance is a pre-computed end-of-day balance snapshot stored in balances_live.
type LiveBalance struct {
	AccountID   string
	BalanceDate time.Time
	Balance     Amount
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
	ID                 string
	FullPath           string
	Type               AccountType
	Product            string
	AccountID          string
	Address            string
	IsPending          bool
	Currency           string
	Exponent           int // e.g. -2 for GBP pence, -5 for high precision
	AnnualInterestRate float64
	OpenedAt           *time.Time
	CreatedAt          time.Time
}

// Movement represents a directed flow of value from one account to another.
// Amount is an integer in the smallest currency unit, stored at the
// higher-precision exponent of the two accounts involved.
type Movement struct {
	ID            string
	BatchID       string
	FromAccountID string
	ToAccountID   string
	Amount        Amount
	Code          string // ISO 20022 BTC mnemonic (DOMAIN:FAMILY:SUBFAMILY)
	Ledger        int32  // partition identifier (TB-inspired)
	PendingID     int64  // two-phase post/void; 0 = N/A (TB-inspired)
	UserData64    int64  // external reference (TB-inspired)
	ValueTime     time.Time
	KnowledgeTime time.Time
	Description   string
	PeriodAnchor  string // "^", "$", or ""
}

// MovementInput describes a movement to be recorded (before it gets a DB id/batch).
type MovementInput struct {
	FromAccountID string
	ToAccountID   string
	Amount        Amount
	Code          string
	Ledger        int32
	PendingID     int64
	UserData64    int64
	Description   string
	KnowledgeTime *time.Time // explicit knowledge time; nil = DEFAULT NOW()
	PeriodAnchor  string     // "^", "$", or ""
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

// BuildFullPath constructs a colon-separated path from components.
// Omits trailing empty components: BuildFullPath("Asset", "Bank", "", "") → "Asset:Bank"
func BuildFullPath(accountType AccountType, product, accountID, address string) string {
	parts := []string{string(accountType), product}
	if accountID != "" || address != "" {
		parts = append(parts, accountID)
	}
	if address != "" {
		parts = append(parts, address)
	}
	return strings.Join(parts, ":")
}

// RebuildFullPath reconstructs FullPath from the account's component fields.
func (a *Account) RebuildFullPath() string {
	a.FullPath = BuildFullPath(a.Type, a.Product, a.AccountID, a.Address)
	return a.FullPath
}
