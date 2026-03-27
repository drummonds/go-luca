package luca

import (
	"testing"
	"time"

	_ "codeberg.org/hum3/go-postgres"
)

func newTestLedger(t *testing.T) *SQLLedger {
	t.Helper()
	l, err := NewLedger(":memory:")
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
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
	if acct.Commodity != "GBP" {
		t.Errorf("Commodity = %q, want %q", acct.Commodity, "GBP")
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
	if acct.GrossInterestRate != 0.0365 {
		t.Errorf("GrossInterestRate = %f, want %f", acct.GrossInterestRate, 0.0365)
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

	if _, err := l.CreateAccount("Asset:Cash", "GBP", -2, 0); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := l.CreateAccount("Asset:Bank", "GBP", -2, 0); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if _, err := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.05); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

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
	if lb.Balance != 100000 {
		t.Errorf("live balance = %d, want 100000", lb.Balance)
	}
}

func TestRecordMovementWithProjectionsNoInterest(t *testing.T) {
	l := newTestLedger(t)

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
		t.Errorf("live balance = %d, want 50000", lb.Balance)
	}
}

// TestIssue1_BalanceAtCrossTimezone demonstrates the root cause of issue #1:
// ncruces/go-sqlite3 stores time.Time as RFC3339 text with timezone offsets
// preserved. SQLite's <= operator does string comparison, which gives wrong
// results when stored and queried timestamps have different timezone offsets.
//
// Example: a deposit at 00:30 BST on June 16 (= 23:30 UTC June 15) is stored
// as "2026-06-16T00:30:00+01:00". Querying with endOfDay UTC June 15
// ("2026-06-15T23:59:59.999999999Z") misses it because "06-16" > "06-15"
// in string comparison, even though the actual instant is earlier.
func TestIssue1_BalanceAtCrossTimezone(t *testing.T) {
	bst := time.FixedZone("BST", 1*60*60) // UTC+1

	tests := []struct {
		name       string
		depositTZ  *time.Location
		queryTZ    *time.Location
		depositDay int
		depositH   int
	}{
		// Deposit at 00:30 BST June 16 = 23:30 UTC June 15.
		// Stored: "2026-06-16T00:30:00+01:00", query: "2026-06-15T23:59:59...Z"
		// String "16" > "15" → movement excluded → balance = 0. BUG.
		{"midnight_crossing_bst_utc", bst, time.UTC, 16, 0},
		// Same-day deposit, cross-tz — works by coincidence (same date string).
		{"same_day_bst_utc", bst, time.UTC, 15, 14},
		// Same timezone — always works.
		{"same_tz_utc", time.UTC, time.UTC, 15, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTestLedger(t)
			equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
			savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)

			depositTime := time.Date(2026, 6, tt.depositDay, tt.depositH, 30, 0, 0, tt.depositTZ)
			if _, err := l.RecordMovement(equity.ID, savings.ID, 100000, CodeBookTransfer, depositTime, "deposit"); err != nil {
				t.Fatalf("RecordMovement: %v", err)
			}

			// Query balance at end of June 15 in query timezone
			endOfDay := time.Date(2026, 6, 15, 23, 59, 59, 999999999, tt.queryTZ)

			bal, err := l.BalanceAt(savings.ID, endOfDay)
			if err != nil {
				t.Fatalf("BalanceAt: %v", err)
			}
			if bal != 100000 {
				var storedTime string
				if err := l.db.QueryRow("SELECT value_time FROM movements LIMIT 1").Scan(&storedTime); err != nil {
					t.Fatalf("Scan storedTime: %v", err)
				}
				t.Errorf("BalanceAt = %d, want 100000\n"+
					"  stored:  %q\n"+
					"  query:   %q\n"+
					"  deposit UTC: %s",
					bal, storedTime,
					endOfDay.Format(time.RFC3339Nano),
					depositTime.UTC().Format(time.RFC3339Nano))
			}
		})
	}
}
