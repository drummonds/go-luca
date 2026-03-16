package luca

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemLedger is a pure Go in-memory Ledger implementation.
// Core operations (accounts, movements, balances) are implemented.
// Advanced features (interest, import/export, projections) return ErrNotImplemented.
type MemLedger struct {
	mu        sync.RWMutex
	accounts  map[string]*Account // id → account
	byPath    map[string]*Account // fullPath → account
	movements []*Movement         // append-only
}

// Compile-time interface check.
var _ Ledger = (*MemLedger)(nil)

// NewMemLedger creates a new empty in-memory ledger.
func NewMemLedger() *MemLedger {
	return &MemLedger{
		accounts: make(map[string]*Account),
		byPath:   make(map[string]*Account),
	}
}

func (m *MemLedger) Close() error { return nil }

func (m *MemLedger) CreateAccount(fullPath string, currency string, exponent int, annualInterestRate float64) (*Account, error) {
	accountType, product, accountID, address, isPending, err := parseFullPath(fullPath)
	if err != nil {
		return nil, fmt.Errorf("parse path: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.byPath[fullPath]; exists {
		return nil, fmt.Errorf("account %q already exists", fullPath)
	}

	a := &Account{
		ID:                 uuid.New().String(),
		FullPath:           fullPath,
		Type:               accountType,
		Product:            product,
		AccountID:          accountID,
		Address:            address,
		IsPending:          isPending,
		Currency:           currency,
		Exponent:           exponent,
		AnnualInterestRate: annualInterestRate,
		CreatedAt:          time.Now(),
	}
	m.accounts[a.ID] = a
	m.byPath[fullPath] = a
	return a, nil
}

func (m *MemLedger) GetAccount(fullPath string) (*Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a := m.byPath[fullPath]
	return a, nil
}

func (m *MemLedger) GetAccountByID(id string) (*Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a := m.accounts[id]
	return a, nil
}

func (m *MemLedger) ListAccounts(typeFilter AccountType) ([]*Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Account
	for _, a := range m.accounts {
		if typeFilter == "" || a.Type == typeFilter {
			result = append(result, a)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].FullPath < result[j].FullPath
	})
	return result, nil
}

func (m *MemLedger) validateSameExponent(fromID, toID string) error {
	from := m.accounts[fromID]
	if from == nil {
		return fmt.Errorf("from account %s not found", fromID)
	}
	to := m.accounts[toID]
	if to == nil {
		return fmt.Errorf("to account %s not found", toID)
	}
	if from.Exponent != to.Exponent {
		return fmt.Errorf("exponent mismatch: from account %q has exponent %d, to account %q has exponent %d",
			from.FullPath, from.Exponent, to.FullPath, to.Exponent)
	}
	return nil
}

func (m *MemLedger) RecordMovement(fromAccountID, toAccountID string, amount Amount, code string, valueTime time.Time, description string) (*Movement, error) {
	if code == "" {
		return nil, fmt.Errorf("movement code is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.validateSameExponent(fromAccountID, toAccountID); err != nil {
		return nil, err
	}

	mov := &Movement{
		ID:            uuid.New().String(),
		BatchID:       uuid.New().String(),
		FromAccountID: fromAccountID,
		ToAccountID:   toAccountID,
		Amount:        amount,
		Code:          code,
		ValueTime:     valueTime,
		KnowledgeTime: time.Now(),
		Description:   description,
	}
	m.movements = append(m.movements, mov)
	return mov, nil
}

func (m *MemLedger) RecordLinkedMovements(movements []MovementInput, valueTime time.Time) (string, error) {
	if len(movements) == 0 {
		return "", fmt.Errorf("no movements to record")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, mi := range movements {
		if err := m.validateSameExponent(mi.FromAccountID, mi.ToAccountID); err != nil {
			return "", err
		}
	}

	batchID := uuid.New().String()

	for _, mi := range movements {
		mov := &Movement{
			ID:            uuid.New().String(),
			BatchID:       batchID,
			FromAccountID: mi.FromAccountID,
			ToAccountID:   mi.ToAccountID,
			Amount:        mi.Amount,
			Code:          mi.Code,
			Ledger:        mi.Ledger,
			PendingID:     mi.PendingID,
			UserData64:    mi.UserData64,
			ValueTime:     valueTime,
			KnowledgeTime: time.Now(),
			Description:   mi.Description,
		}
		m.movements = append(m.movements, mov)
	}
	return batchID, nil
}

func (m *MemLedger) Balance(accountID string) (Amount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var balance Amount
	for _, mov := range m.movements {
		if mov.ToAccountID == accountID {
			balance += mov.Amount
		}
		if mov.FromAccountID == accountID {
			balance -= mov.Amount
		}
	}
	return balance, nil
}

func (m *MemLedger) BalanceAt(accountID string, at time.Time) (Amount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var balance Amount
	for _, mov := range m.movements {
		if mov.ValueTime.After(at) {
			continue
		}
		if mov.ToAccountID == accountID {
			balance += mov.Amount
		}
		if mov.FromAccountID == accountID {
			balance -= mov.Amount
		}
	}
	return balance, nil
}

func (m *MemLedger) DailyBalances(accountID string, from, to time.Time) ([]DailyBalance, error) {
	var result []DailyBalance
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		endOfDay := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 999999999, d.Location())
		bal, err := m.BalanceAt(accountID, endOfDay)
		if err != nil {
			return nil, fmt.Errorf("balance at %s: %w", d.Format("2006-01-02"), err)
		}
		result = append(result, DailyBalance{
			Date:    time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location()),
			Balance: bal,
		})
	}
	return result, nil
}

// --- Stubbed methods ---

func (m *MemLedger) RecordMovementWithProjections(fromAccountID, toAccountID string, amount Amount, code string, valueTime time.Time, description string) (*Movement, error) {
	return nil, ErrNotImplemented
}

func (m *MemLedger) BalanceByPath(pathPrefix string, at time.Time) (Amount, int, error) {
	return 0, 0, ErrNotImplemented
}

func (m *MemLedger) GetLiveBalance(accountID string, date time.Time) (*LiveBalance, error) {
	return nil, ErrNotImplemented
}

func (m *MemLedger) EnsureInterestAccounts() error {
	return ErrNotImplemented
}

func (m *MemLedger) CalculateDailyInterest(accountID string, date time.Time) (*InterestResult, error) {
	return nil, ErrNotImplemented
}

func (m *MemLedger) RunDailyInterest(date time.Time) ([]InterestResult, error) {
	return nil, ErrNotImplemented
}

func (m *MemLedger) RunInterestForPeriod(from, to time.Time) ([]InterestResult, error) {
	return nil, ErrNotImplemented
}

func (m *MemLedger) ListMovements() ([]MovementWithPaths, error) {
	return nil, ErrNotImplemented
}

func (m *MemLedger) Export(w io.Writer) error {
	return ErrNotImplemented
}

func (m *MemLedger) Import(r io.Reader, opts *ImportOptions) error {
	return ErrNotImplemented
}

func (m *MemLedger) ImportString(s string, opts *ImportOptions) error {
	return ErrNotImplemented
}
