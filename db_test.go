package luca

import (
	"testing"
	"time"

	_ "github.com/drummonds/go-postgres"
)

func newTestLedger(t *testing.T) *SQLLedger {
	t.Helper()
	l, err := NewLedger(":memory:")
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestCreateAccount(t *testing.T) {
	l := newTestLedger(t)

	acct, err := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acct.ID == "" {
		t.Error("expected non-empty ID")
	}
	if acct.FullPath != "Asset:Cash" {
		t.Errorf("FullPath = %q, want %q", acct.FullPath, "Asset:Cash")
	}
	if acct.Type != AccountTypeAsset {
		t.Errorf("Type = %q, want %q", acct.Type, AccountTypeAsset)
	}
	if acct.Product != "Cash" {
		t.Errorf("Product = %q, want %q", acct.Product, "Cash")
	}
	if acct.Currency != "GBP" {
		t.Errorf("Currency = %q, want %q", acct.Currency, "GBP")
	}
	if acct.Exponent != -2 {
		t.Errorf("Exponent = %d, want -2", acct.Exponent)
	}
}

func TestCreateAccountWithInterestRate(t *testing.T) {
	l := newTestLedger(t)

	acct, err := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.0365)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acct.AnnualInterestRate != 0.0365 {
		t.Errorf("AnnualInterestRate = %f, want %f", acct.AnnualInterestRate, 0.0365)
	}
}

func TestCreateDuplicateAccount(t *testing.T) {
	l := newTestLedger(t)

	_, err := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		t.Fatalf("first CreateAccount: %v", err)
	}
	_, err = l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err == nil {
		t.Fatal("expected error for duplicate account, got nil")
	}
}

func TestGetAccount(t *testing.T) {
	l := newTestLedger(t)

	_, err := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	acct, err := l.GetAccount("Asset:Cash")
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if acct == nil {
		t.Fatal("expected account, got nil")
	}
	if acct.FullPath != "Asset:Cash" {
		t.Errorf("FullPath = %q, want %q", acct.FullPath, "Asset:Cash")
	}

	// Non-existent account
	acct, err = l.GetAccount("Asset:NonExistent")
	if err != nil {
		t.Fatalf("GetAccount non-existent: %v", err)
	}
	if acct != nil {
		t.Errorf("expected nil for non-existent account, got %v", acct)
	}
}

func TestGetAccountByID(t *testing.T) {
	l := newTestLedger(t)

	created, err := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	acct, err := l.GetAccountByID(created.ID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if acct.FullPath != "Asset:Cash" {
		t.Errorf("FullPath = %q, want %q", acct.FullPath, "Asset:Cash")
	}
}

func TestListAccounts(t *testing.T) {
	l := newTestLedger(t)

	l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	l.CreateAccount("Asset:Bank", "GBP", -2, 0)
	l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.05)

	// All accounts
	all, err := l.ListAccounts("")
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("got %d accounts, want 3", len(all))
	}

	// Filter by type
	assets, err := l.ListAccounts(AccountTypeAsset)
	if err != nil {
		t.Fatalf("ListAccounts Asset: %v", err)
	}
	if len(assets) != 2 {
		t.Errorf("got %d asset accounts, want 2", len(assets))
	}
}

func TestRecordMovement(t *testing.T) {
	l := newTestLedger(t)

	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	m, err := l.RecordMovement(equity.ID, cash.ID, 20000, CodeBookTransfer, now, "Initial capital")
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if m.ID == "" {
		t.Error("expected non-empty movement ID")
	}
	if m.BatchID == "" {
		t.Error("expected non-empty batch ID")
	}
	if m.Amount != 20000 {
		t.Errorf("Amount = %d, want 20000", m.Amount)
	}
	if m.FromAccountID != equity.ID {
		t.Errorf("FromAccountID = %s, want %s", m.FromAccountID, equity.ID)
	}
	if m.ToAccountID != cash.ID {
		t.Errorf("ToAccountID = %s, want %s", m.ToAccountID, cash.ID)
	}
}

func TestRecordLinkedMovements(t *testing.T) {
	l := newTestLedger(t)

	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	purchases, _ := l.CreateAccount("Expense:Purchases", "GBP", -2, 0)
	vat, _ := l.CreateAccount("Asset:VATInput", "GBP", -2, 0)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	batchID, err := l.RecordLinkedMovements([]MovementInput{
		{FromAccountID: cash.ID, ToAccountID: purchases.ID, Amount: 50000, Code: CodeBookTransfer, Description: "Office supplies"},
		{FromAccountID: cash.ID, ToAccountID: vat.ID, Amount: 10000, Code: CodeBookTransfer, Description: "VAT"},
	}, now)
	if err != nil {
		t.Fatalf("RecordLinkedMovements: %v", err)
	}
	if batchID == "" {
		t.Error("expected non-empty batch ID")
	}
}

func TestRecordLinkedMovementsEmpty(t *testing.T) {
	l := newTestLedger(t)

	_, err := l.RecordLinkedMovements(nil, time.Now())
	if err == nil {
		t.Fatal("expected error for empty movements, got nil")
	}
}

func TestRecordMovementWithProjections(t *testing.T) {
	l := newTestLedger(t)
	if err := l.EnsureInterestAccounts(); err != nil {
		t.Fatalf("EnsureInterestAccounts: %v", err)
	}

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.05)
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	m, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 100000, CodeBookTransfer, now, "deposit")
	if err != nil {
		t.Fatalf("RecordMovementWithProjections: %v", err)
	}
	if m.ID == "" {
		t.Error("expected non-empty movement ID")
	}
	if m.Code != CodeBookTransfer {
		t.Errorf("Code = %q, want %q", m.Code, CodeBookTransfer)
	}

	// Verify live balance was created
	lb, err := l.GetLiveBalance(savings.ID, now)
	if err != nil {
		t.Fatalf("GetLiveBalance: %v", err)
	}
	if lb == nil {
		t.Fatal("expected live balance, got nil")
	}
	// Balance should include both the deposit and interest accrual
	if lb.Balance <= 100000 {
		t.Errorf("live balance = %d, want > 100000 (should include interest)", lb.Balance)
	}
}

func TestRecordMovementWithProjectionsNoInterest(t *testing.T) {
	l := newTestLedger(t)
	if err := l.EnsureInterestAccounts(); err != nil {
		t.Fatalf("EnsureInterestAccounts: %v", err)
	}

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0) // 0% rate
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	_, err := l.RecordMovementWithProjections(equity.ID, cash.ID, 50000, CodeBookTransfer, now, "deposit")
	if err != nil {
		t.Fatalf("RecordMovementWithProjections: %v", err)
	}

	lb, err := l.GetLiveBalance(cash.ID, now)
	if err != nil {
		t.Fatalf("GetLiveBalance: %v", err)
	}
	if lb == nil {
		t.Fatal("expected live balance, got nil")
	}
	if lb.Balance != 50000 {
		t.Errorf("live balance = %d, want 50000 (no interest)", lb.Balance)
	}
}

func TestCompoundMovementUpsertsInterest(t *testing.T) {
	l := newTestLedger(t)
	if err := l.EnsureInterestAccounts(); err != nil {
		t.Fatalf("EnsureInterestAccounts: %v", err)
	}

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.05)
	day := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	// First deposit
	_, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 100000, CodeBookTransfer, day, "deposit 1")
	if err != nil {
		t.Fatalf("first deposit: %v", err)
	}

	// Second deposit same day — should upsert interest accrual, not duplicate
	_, err = l.RecordMovementWithProjections(equity.ID, savings.ID, 50000, CodeBookTransfer, day.Add(2*time.Hour), "deposit 2")
	if err != nil {
		t.Fatalf("second deposit: %v", err)
	}

	// Count interest accrual movements for this account+day
	eod := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 999999999, day.Location())
	bod := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	var count int
	err = l.db.QueryRow(
		`SELECT COUNT(*) FROM movements
		 WHERE to_account_id = $1 AND code = $2
		   AND value_time >= $3 AND value_time <= $4`,
		savings.ID, CodeInterestAccrual, bod, eod,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count accruals: %v", err)
	}
	if count != 1 {
		t.Errorf("interest accrual count = %d, want 1 (should be upserted, not duplicated)", count)
	}
}
