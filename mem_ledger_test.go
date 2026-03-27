package luca

import (
	"testing"
	"time"
)

func TestMemLedgerCreateAccount(t *testing.T) {
	m := NewMemLedger()
	defer func() { _ = m.Close() }()

	acct, err := m.CreateAccount("Asset:Cash", "GBP", -2, 0)
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
}

func TestMemLedgerDuplicateAccount(t *testing.T) {
	m := NewMemLedger()

	_, err := m.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		t.Fatalf("first CreateAccount: %v", err)
	}
	_, err = m.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err == nil {
		t.Fatal("expected error for duplicate account")
	}
}

func TestMemLedgerGetAccount(t *testing.T) {
	m := NewMemLedger()

	if _, err := m.CreateAccount("Asset:Cash", "GBP", -2, 0); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	acct, err := m.GetAccount("Asset:Cash")
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if acct == nil || acct.FullPath != "Asset:Cash" {
		t.Fatalf("expected Asset:Cash, got %v", acct)
	}

	acct, err = m.GetAccount("Asset:NonExistent")
	if err != nil {
		t.Fatalf("GetAccount non-existent: %v", err)
	}
	if acct != nil {
		t.Errorf("expected nil, got %v", acct)
	}
}

func TestMemLedgerGetAccountByID(t *testing.T) {
	m := NewMemLedger()

	created, _ := m.CreateAccount("Asset:Cash", "GBP", -2, 0)

	acct, err := m.GetAccountByID(created.ID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if acct.FullPath != "Asset:Cash" {
		t.Errorf("FullPath = %q, want %q", acct.FullPath, "Asset:Cash")
	}

	acct, _ = m.GetAccountByID("nonexistent-uuid")
	if acct != nil {
		t.Errorf("expected nil for non-existent ID")
	}
}

func TestMemLedgerListAccounts(t *testing.T) {
	m := NewMemLedger()

	if _, err := m.CreateAccount("Asset:Cash", "GBP", -2, 0); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := m.CreateAccount("Asset:Bank", "GBP", -2, 0); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := m.CreateAccount("Liability:Savings:0001", "GBP", -2, 0); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	all, _ := m.ListAccounts("")
	if len(all) != 3 {
		t.Errorf("got %d accounts, want 3", len(all))
	}

	assets, _ := m.ListAccounts(AccountTypeAsset)
	if len(assets) != 2 {
		t.Errorf("got %d asset accounts, want 2", len(assets))
	}
}

func TestMemLedgerRecordMovement(t *testing.T) {
	m := NewMemLedger()

	cash, _ := m.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := m.CreateAccount("Equity:Capital", "GBP", -2, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	mov, err := m.RecordMovement(equity.ID, cash.ID, 20000, CodeBookTransfer, now, "Initial capital")
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if mov.ID == "" {
		t.Error("expected non-empty movement ID")
	}
	if mov.Amount != 20000 {
		t.Errorf("Amount = %d, want 20000", mov.Amount)
	}
}

func TestMemLedgerRecordMovementExponentMismatch(t *testing.T) {
	m := NewMemLedger()

	cash, _ := m.CreateAccount("Asset:Cash", "GBP", -2, 0)
	precise, _ := m.CreateAccount("Asset:Precise", "GBP", -5, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	_, err := m.RecordMovement(cash.ID, precise.ID, 100, CodeBookTransfer, now, "mismatch")
	if err == nil {
		t.Fatal("expected exponent mismatch error")
	}
}

func TestMemLedgerRecordLinkedMovements(t *testing.T) {
	m := NewMemLedger()

	cash, _ := m.CreateAccount("Asset:Cash", "GBP", -2, 0)
	purchases, _ := m.CreateAccount("Expense:Purchases", "GBP", -2, 0)
	vat, _ := m.CreateAccount("Asset:VATInput", "GBP", -2, 0)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	batchID, err := m.RecordLinkedMovements([]MovementInput{
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

func TestMemLedgerRecordLinkedMovementsEmpty(t *testing.T) {
	m := NewMemLedger()

	_, err := m.RecordLinkedMovements(nil, time.Now())
	if err == nil {
		t.Fatal("expected error for empty movements")
	}
}

func TestMemLedgerBalance(t *testing.T) {
	m := NewMemLedger()

	cash, _ := m.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := m.CreateAccount("Equity:Capital", "GBP", -2, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	if _, err := m.RecordMovement(equity.ID, cash.ID, 20000, CodeBookTransfer, now, "deposit"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if _, err := m.RecordMovement(equity.ID, cash.ID, 5000, CodeBookTransfer, now, "more"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	bal, err := m.Balance(cash.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal != 25000 {
		t.Errorf("Balance = %d, want 25000", bal)
	}

	eqBal, _ := m.Balance(equity.ID)
	if eqBal != -25000 {
		t.Errorf("Equity Balance = %d, want -25000", eqBal)
	}
}

func TestMemLedgerBalanceAt(t *testing.T) {
	m := NewMemLedger()

	cash, _ := m.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := m.CreateAccount("Equity:Capital", "GBP", -2, 0)

	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	jan2 := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	if _, err := m.RecordMovement(equity.ID, cash.ID, 10000, CodeBookTransfer, jan1, "first"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if _, err := m.RecordMovement(equity.ID, cash.ID, 5000, CodeBookTransfer, jan2, "second"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	bal, err := m.BalanceAt(cash.ID, jan1)
	if err != nil {
		t.Fatalf("BalanceAt: %v", err)
	}
	if bal != 10000 {
		t.Errorf("BalanceAt jan1 = %d, want 10000", bal)
	}

	bal2, _ := m.BalanceAt(cash.ID, jan2)
	if bal2 != 15000 {
		t.Errorf("BalanceAt jan2 = %d, want 15000", bal2)
	}
}

func TestMemLedgerDailyBalances(t *testing.T) {
	m := NewMemLedger()

	cash, _ := m.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := m.CreateAccount("Equity:Capital", "GBP", -2, 0)

	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	jan2 := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	if _, err := m.RecordMovement(equity.ID, cash.ID, 10000, CodeBookTransfer, jan1, "first"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if _, err := m.RecordMovement(equity.ID, cash.ID, 5000, CodeBookTransfer, jan2, "second"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	dailies, err := m.DailyBalances(cash.ID, from, to)
	if err != nil {
		t.Fatalf("DailyBalances: %v", err)
	}
	if len(dailies) != 3 {
		t.Fatalf("got %d daily balances, want 3", len(dailies))
	}
	if dailies[0].Balance != 10000 {
		t.Errorf("day 1 balance = %d, want 10000", dailies[0].Balance)
	}
	if dailies[1].Balance != 15000 {
		t.Errorf("day 2 balance = %d, want 15000", dailies[1].Balance)
	}
	if dailies[2].Balance != 15000 {
		t.Errorf("day 3 balance = %d, want 15000", dailies[2].Balance)
	}
}

func TestMemLedgerStubs(t *testing.T) {
	m := NewMemLedger()

	if _, err := m.RecordMovementWithProjections("", "", 0, CodeBookTransfer, time.Now(), ""); err != ErrNotImplemented {
		t.Errorf("RecordMovementWithProjections = %v, want ErrNotImplemented", err)
	}
	if _, _, err := m.BalanceByPath("", time.Now()); err != ErrNotImplemented {
		t.Errorf("BalanceByPath = %v, want ErrNotImplemented", err)
	}
	if _, err := m.GetLiveBalance("", time.Now()); err != ErrNotImplemented {
		t.Errorf("GetLiveBalance = %v, want ErrNotImplemented", err)
	}
	if _, err := m.ListMovements(); err != ErrNotImplemented {
		t.Errorf("ListMovements = %v, want ErrNotImplemented", err)
	}
	if err := m.Export(nil); err != ErrNotImplemented {
		t.Errorf("Export = %v, want ErrNotImplemented", err)
	}
	if err := m.Import(nil, nil); err != ErrNotImplemented {
		t.Errorf("Import = %v, want ErrNotImplemented", err)
	}
	if err := m.ImportString("", nil); err != ErrNotImplemented {
		t.Errorf("ImportString = %v, want ErrNotImplemented", err)
	}
}

// TestMemLedgerInterfaceCompliance verifies both backends satisfy Ledger.
func TestMemLedgerInterfaceCompliance(t *testing.T) {
	var _ Ledger = (*MemLedger)(nil)
	var _ Ledger = (*SQLLedger)(nil)
}
