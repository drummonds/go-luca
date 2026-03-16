package api_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/drummonds/go-luca"
	"github.com/drummonds/go-luca/api"
	_ "github.com/drummonds/go-postgres"
)

func setup(t *testing.T) luca.Ledger {
	t.Helper()
	l, err := luca.NewLedger(":memory:")
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	srv := api.NewServer(l)
	ts := httptest.NewServer(srv)
	t.Cleanup(func() {
		ts.Close()
		l.Close()
	})
	return api.NewClient(ts.URL)
}

func TestCreateAndGetAccount(t *testing.T) {
	c := setup(t)

	acct, err := c.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acct.ID == "" {
		t.Error("expected non-empty ID")
	}
	if acct.FullPath != "Asset:Cash" {
		t.Errorf("FullPath = %q, want %q", acct.FullPath, "Asset:Cash")
	}

	got, err := c.GetAccount("Asset:Cash")
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if got == nil || got.ID != acct.ID {
		t.Errorf("GetAccount returned %v, want ID %s", got, acct.ID)
	}

	// Non-existent returns nil
	missing, err := c.GetAccount("Asset:NonExistent")
	if err != nil {
		t.Fatalf("GetAccount non-existent: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil, got %v", missing)
	}
}

func TestGetAccountByID(t *testing.T) {
	c := setup(t)

	created, _ := c.CreateAccount("Asset:Cash", "GBP", -2, 0)
	got, err := c.GetAccountByID(created.ID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	if got.FullPath != "Asset:Cash" {
		t.Errorf("FullPath = %q, want %q", got.FullPath, "Asset:Cash")
	}

	missing, err := c.GetAccountByID("nonexistent-uuid")
	if err != nil {
		t.Fatalf("GetAccountByID non-existent: %v", err)
	}
	if missing != nil {
		t.Errorf("expected nil, got %v", missing)
	}
}

func TestListAccounts(t *testing.T) {
	c := setup(t)

	c.CreateAccount("Asset:Cash", "GBP", -2, 0)
	c.CreateAccount("Asset:Bank", "GBP", -2, 0)
	c.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)

	all, err := c.ListAccounts("")
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("got %d accounts, want 3", len(all))
	}

	assets, err := c.ListAccounts(luca.AccountTypeAsset)
	if err != nil {
		t.Fatalf("ListAccounts Asset: %v", err)
	}
	if len(assets) != 2 {
		t.Errorf("got %d asset accounts, want 2", len(assets))
	}
}

func TestRecordMovementAndBalance(t *testing.T) {
	c := setup(t)

	cash, _ := c.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := c.CreateAccount("Equity:Capital", "GBP", -2, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	mov, err := c.RecordMovement(equity.ID, cash.ID, 20000, luca.CodeBookTransfer, now, "Initial capital")
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if mov.ID == "" {
		t.Error("expected non-empty movement ID")
	}
	if mov.Amount != 20000 {
		t.Errorf("Amount = %d, want 20000", mov.Amount)
	}

	bal, err := c.Balance(cash.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal != 20000 {
		t.Errorf("Balance = %d, want 20000", bal)
	}
}

func TestRecordLinkedMovements(t *testing.T) {
	c := setup(t)

	cash, _ := c.CreateAccount("Asset:Cash", "GBP", -2, 0)
	purchases, _ := c.CreateAccount("Expense:Purchases", "GBP", -2, 0)
	vat, _ := c.CreateAccount("Asset:VATInput", "GBP", -2, 0)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	batchID, err := c.RecordLinkedMovements([]luca.MovementInput{
		{FromAccountID: cash.ID, ToAccountID: purchases.ID, Amount: 50000, Code: luca.CodeBookTransfer, Description: "Office supplies"},
		{FromAccountID: cash.ID, ToAccountID: vat.ID, Amount: 10000, Code: luca.CodeBookTransfer, Description: "VAT"},
	}, now)
	if err != nil {
		t.Fatalf("RecordLinkedMovements: %v", err)
	}
	if batchID == "" {
		t.Error("expected non-empty batch ID")
	}

	bal, _ := c.Balance(cash.ID)
	if bal != -60000 {
		t.Errorf("Cash balance = %d, want -60000", bal)
	}
}

func TestBalanceAt(t *testing.T) {
	c := setup(t)

	cash, _ := c.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := c.CreateAccount("Equity:Capital", "GBP", -2, 0)

	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	jan2 := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	c.RecordMovement(equity.ID, cash.ID, 10000, luca.CodeBookTransfer, jan1, "first")
	c.RecordMovement(equity.ID, cash.ID, 5000, luca.CodeBookTransfer, jan2, "second")

	bal, err := c.BalanceAt(cash.ID, jan1)
	if err != nil {
		t.Fatalf("BalanceAt: %v", err)
	}
	if bal != 10000 {
		t.Errorf("BalanceAt jan1 = %d, want 10000", bal)
	}

	bal2, _ := c.BalanceAt(cash.ID, jan2)
	if bal2 != 15000 {
		t.Errorf("BalanceAt jan2 = %d, want 15000", bal2)
	}
}

func TestDailyBalances(t *testing.T) {
	c := setup(t)

	cash, _ := c.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := c.CreateAccount("Equity:Capital", "GBP", -2, 0)

	jan1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	jan2 := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	c.RecordMovement(equity.ID, cash.ID, 10000, luca.CodeBookTransfer, jan1, "first")
	c.RecordMovement(equity.ID, cash.ID, 5000, luca.CodeBookTransfer, jan2, "second")

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	dailies, err := c.DailyBalances(cash.ID, from, to)
	if err != nil {
		t.Fatalf("DailyBalances: %v", err)
	}
	if len(dailies) != 2 {
		t.Fatalf("got %d daily balances, want 2", len(dailies))
	}
	if dailies[0].Balance != 10000 {
		t.Errorf("day 1 = %d, want 10000", dailies[0].Balance)
	}
	if dailies[1].Balance != 15000 {
		t.Errorf("day 2 = %d, want 15000", dailies[1].Balance)
	}
}
