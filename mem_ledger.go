package luca

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// MemLedger is a pure Go in-memory Ledger implementation.
// Core operations (accounts, movements, balances) are implemented.
// Advanced features (interest, import/export, projections) return ErrNotImplemented.
type MemLedger struct {
	mu        sync.RWMutex
	accounts  []*Account          // append-only, index = ID-1
	byPath    map[string]*Account // fullPath → account
	movements []*Movement         // append-only
	nextBatch int64
	nextMovID int64
}

// Compile-time interface check.
var _ Ledger = (*MemLedger)(nil)

// NewMemLedger creates a new empty in-memory ledger.
func NewMemLedger() *MemLedger {
	return &MemLedger{
		byPath:    make(map[string]*Account),
		nextBatch: 1,
		nextMovID: 1,
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
		ID:                 int64(len(m.accounts) + 1),
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
	m.accounts = append(m.accounts, a)
	m.byPath[fullPath] = a
	return a, nil
}

func (m *MemLedger) GetAccount(fullPath string) (*Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a := m.byPath[fullPath]
	return a, nil
}

func (m *MemLedger) GetAccountByID(id int64) (*Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if id < 1 || int(id) > len(m.accounts) {
		return nil, nil
	}
	return m.accounts[id-1], nil
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

func (m *MemLedger) validateSameExponent(fromID, toID int64) error {
	from, _ := m.getAccountByIDLocked(fromID)
	if from == nil {
		return fmt.Errorf("from account %d not found", fromID)
	}
	to, _ := m.getAccountByIDLocked(toID)
	if to == nil {
		return fmt.Errorf("to account %d not found", toID)
	}
	if from.Exponent != to.Exponent {
		return fmt.Errorf("exponent mismatch: from account %q has exponent %d, to account %q has exponent %d",
			from.FullPath, from.Exponent, to.FullPath, to.Exponent)
	}
	return nil
}

// getAccountByIDLocked requires caller to hold at least RLock.
func (m *MemLedger) getAccountByIDLocked(id int64) (*Account, error) {
	if id < 1 || int(id) > len(m.accounts) {
		return nil, nil
	}
	return m.accounts[id-1], nil
}

func (m *MemLedger) RecordMovement(fromAccountID, toAccountID int64, amount int64, valueTime time.Time, description string) (*Movement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.validateSameExponent(fromAccountID, toAccountID); err != nil {
		return nil, err
	}

	batchID := m.nextBatch
	m.nextBatch++
	movID := m.nextMovID
	m.nextMovID++

	mov := &Movement{
		ID:            movID,
		BatchID:       batchID,
		FromAccountID: fromAccountID,
		ToAccountID:   toAccountID,
		Amount:        amount,
		ValueTime:     valueTime,
		KnowledgeTime: time.Now(),
		Description:   description,
	}
	m.movements = append(m.movements, mov)
	return mov, nil
}

func (m *MemLedger) RecordLinkedMovements(movements []MovementInput, valueTime time.Time) (int64, error) {
	if len(movements) == 0 {
		return 0, fmt.Errorf("no movements to record")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, mi := range movements {
		if err := m.validateSameExponent(mi.FromAccountID, mi.ToAccountID); err != nil {
			return 0, err
		}
	}

	batchID := m.nextBatch
	m.nextBatch++

	for _, mi := range movements {
		movID := m.nextMovID
		m.nextMovID++
		mov := &Movement{
			ID:            movID,
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

func (m *MemLedger) Balance(accountID int64) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var balance int64
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

func (m *MemLedger) BalanceAt(accountID int64, at time.Time) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var balance int64
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

func (m *MemLedger) DailyBalances(accountID int64, from, to time.Time) ([]DailyBalance, error) {
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

func (m *MemLedger) RecordMovementWithProjections(fromAccountID, toAccountID int64, amount int64, valueTime time.Time, description string) (*Movement, error) {
	return nil, ErrNotImplemented
}

func (m *MemLedger) BalanceByPath(pathPrefix string, at time.Time) (int64, int, error) {
	return 0, 0, ErrNotImplemented
}

func (m *MemLedger) GetLiveBalance(accountID int64, date time.Time) (*LiveBalance, error) {
	return nil, ErrNotImplemented
}

func (m *MemLedger) EnsureInterestAccounts() error {
	return ErrNotImplemented
}

func (m *MemLedger) CalculateDailyInterest(accountID int64, date time.Time) (*InterestResult, error) {
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
