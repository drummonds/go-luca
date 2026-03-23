package luca

import (
	"testing"
	"time"

	_ "github.com/drummonds/go-postgres"
)

func TestBalanceSimple(t *testing.T) {
	l := newTestLedger(t)

	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Equity -> Cash 200.00 GBP = 20000 pence
	if _, err := l.RecordMovement(equity.ID, cash.ID, 20000, CodeBookTransfer, now, "Initial capital"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	// Cash balance should be +20000 (inflow)
	cashBal, err := l.Balance(cash.ID)
	if err != nil {
		t.Fatalf("Balance cash: %v", err)
	}
	if cashBal != 20000 {
		t.Errorf("cash balance = %d, want 20000", cashBal)
	}

	// Equity balance should be -20000 (outflow)
	equityBal, err := l.Balance(equity.ID)
	if err != nil {
		t.Fatalf("Balance equity: %v", err)
	}
	if equityBal != -20000 {
		t.Errorf("equity balance = %d, want -20000", equityBal)
	}
}

func TestBalanceZeroForNewAccount(t *testing.T) {
	l := newTestLedger(t)

	acct, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	bal, err := l.Balance(acct.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal != 0 {
		t.Errorf("balance = %d, want 0", bal)
	}
}

func TestBalanceMultipleMovements(t *testing.T) {
	l := newTestLedger(t)

	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	expenses, _ := l.CreateAccount("Expense:Rent", "GBP", -2, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	if _, err := l.RecordMovement(equity.ID, cash.ID, 100000, CodeBookTransfer, now, "Capital"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if _, err := l.RecordMovement(cash.ID, expenses.ID, 30000, CodeBookTransfer, now.Add(time.Hour), "Rent"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	// Cash: +100000 - 30000 = 70000 (700.00)
	cashBal, err := l.Balance(cash.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if cashBal != 70000 {
		t.Errorf("cash balance = %d, want 70000", cashBal)
	}
}

func TestBalanceAt(t *testing.T) {
	l := newTestLedger(t)

	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	expenses, _ := l.CreateAccount("Expense:Rent", "GBP", -2, 0)

	day1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

	if _, err := l.RecordMovement(equity.ID, cash.ID, 100000, CodeBookTransfer, day1, "Capital"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if _, err := l.RecordMovement(cash.ID, expenses.ID, 30000, CodeBookTransfer, day2, "Rent"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	// Balance at end of day 1: should be 100000 (rent not yet paid)
	endOfDay1 := time.Date(2026, 1, 1, 23, 59, 59, 999999999, time.UTC)
	bal, err := l.BalanceAt(cash.ID, endOfDay1)
	if err != nil {
		t.Fatalf("BalanceAt: %v", err)
	}
	if bal != 100000 {
		t.Errorf("balance at day 1 = %d, want 100000", bal)
	}

	// Balance at end of day 2: should be 70000
	endOfDay2 := time.Date(2026, 1, 2, 23, 59, 59, 999999999, time.UTC)
	bal, err = l.BalanceAt(cash.ID, endOfDay2)
	if err != nil {
		t.Fatalf("BalanceAt: %v", err)
	}
	if bal != 70000 {
		t.Errorf("balance at day 2 = %d, want 70000", bal)
	}
}

func TestBalanceByPathPrefix(t *testing.T) {
	l := newTestLedger(t)

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings1, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)
	savings2, _ := l.CreateAccount("Liability:Savings:0002", "GBP", -2, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	if _, err := l.RecordMovement(equity.ID, savings1.ID, 100000, CodeBookTransfer, now, "Deposit 1"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if _, err := l.RecordMovement(equity.ID, savings2.ID, 200000, CodeBookTransfer, now, "Deposit 2"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	// Total liabilities
	endOfDay := time.Date(2026, 1, 1, 23, 59, 59, 999999999, time.UTC)
	bal, exp, err := l.BalanceByPath("Liability", endOfDay)
	if err != nil {
		t.Fatalf("BalanceByPath: %v", err)
	}
	if bal != 300000 {
		t.Errorf("total liabilities = %d, want 300000", bal)
	}
	if exp != -2 {
		t.Errorf("exponent = %d, want -2", exp)
	}

	// Specific account via path
	bal, _, err = l.BalanceByPath("Liability:Savings:0001", endOfDay)
	if err != nil {
		t.Fatalf("BalanceByPath specific: %v", err)
	}
	if bal != 100000 {
		t.Errorf("savings 0001 = %d, want 100000", bal)
	}
}

func TestDailyBalances(t *testing.T) {
	l := newTestLedger(t)

	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)

	day1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)

	if _, err := l.RecordMovement(equity.ID, cash.ID, 100000, CodeBookTransfer, day1, "Capital"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if _, err := l.RecordMovement(equity.ID, cash.ID, 50000, CodeBookTransfer, day3, "More capital"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	balances, err := l.DailyBalances(cash.ID, from, to)
	if err != nil {
		t.Fatalf("DailyBalances: %v", err)
	}
	if len(balances) != 3 {
		t.Fatalf("got %d daily balances, want 3", len(balances))
	}
	// Day 1: 100000, Day 2: 100000, Day 3: 150000
	expected := []Amount{100000, 100000, 150000}
	for i, want := range expected {
		if balances[i].Balance != want {
			t.Errorf("day %d balance = %d, want %d", i+1, balances[i].Balance, want)
		}
	}
}

// TestCrossExponentMovementRejected verifies that movements between accounts
// with different exponents are rejected (cross-exponent = commodity conversion).
func TestCrossExponentMovementRejected(t *testing.T) {
	l := newTestLedger(t)

	std, _ := l.CreateAccount("Asset:Standard", "GBP", -2, 0)
	hip, _ := l.CreateAccount("Asset:HiPrec", "GBPHP", -5, 0) // different commodity → different exponent

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_, err := l.RecordMovement(std.ID, hip.ID, 100000, CodeBookTransfer, now, "Should fail")
	if err == nil {
		t.Fatal("expected error for cross-exponent movement, got nil")
	}
}

// TestCrossExponentLinkedMovementRejected verifies that linked movements
// between accounts with different exponents are also rejected.
func TestCrossExponentLinkedMovementRejected(t *testing.T) {
	l := newTestLedger(t)

	std, _ := l.CreateAccount("Asset:Standard", "GBP", -2, 0)
	hip, _ := l.CreateAccount("Asset:HiPrec", "GBPHP", -5, 0) // different commodity → different exponent

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	_, err := l.RecordLinkedMovements([]MovementInput{
		{FromAccountID: std.ID, ToAccountID: hip.ID, Amount: 100000, Code: CodeBookTransfer, Description: "Should fail"},
	}, now)
	if err == nil {
		t.Fatal("expected error for cross-exponent linked movement, got nil")
	}
}

// TestMixedExponentBalanceByPath verifies BalanceByPath scales correctly when
// reporting across accounts with different exponents (different ledger partitions).
func TestMixedExponentBalanceByPath(t *testing.T) {
	l := newTestLedger(t)

	// Two Asset accounts at different exponents (different commodities), each funded within its own exponent group
	standard, _ := l.CreateAccount("Asset:Standard", "GBP", -2, 0)
	hiPrec, _ := l.CreateAccount("Asset:HiPrec", "GBPHP", -5, 0)
	equity2, _ := l.CreateAccount("Equity:Capital2", "GBP", -2, 0)   // funds standard
	equity5, _ := l.CreateAccount("Equity:Capital5", "GBPHP", -5, 0) // funds hiPrec
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// 1000.00 into standard: equity2(-2) → standard(-2), same exponent
	if _, err := l.RecordMovement(equity2.ID, standard.ID, 100000, CodeBookTransfer, now, "Deposit std"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	// 1.00000 into hiPrec: equity5(-5) → hiPrec(-5), same exponent
	if _, err := l.RecordMovement(equity5.ID, hiPrec.ID, 100000, CodeBookTransfer, now, "Deposit hip"); err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	endOfDay := time.Date(2026, 1, 1, 23, 59, 59, 999999999, time.UTC)

	// BalanceByPath("Asset") reports at min exponent = -5
	// standard: 100000 at -2, scaled to -5 = 10000000000? No:
	//   IntToDecimal(100000, -2) = 1000.00
	//   DecimalToInt(1000.00, -5) = 100000000
	// hiPrec: 100000 at -5 = 100000
	// Total: 100000000 + 100000 = 100100000 (= 1001.00000)
	bal, exp, err := l.BalanceByPath("Asset", endOfDay)
	if err != nil {
		t.Fatalf("BalanceByPath: %v", err)
	}
	if exp != -5 {
		t.Errorf("report exponent = %d, want -5", exp)
	}
	want := Amount(100100000) // 1001.00000 at -5
	if bal != want {
		t.Errorf("BalanceByPath(Asset) = %d, want %d (1001.00000)", bal, want)
	}

	// Individual balances are still correct at their own exponents
	stdBal, _ := l.Balance(standard.ID)
	if stdBal != 100000 {
		t.Errorf("std balance = %d, want 100000", stdBal)
	}
	hipBal, _ := l.Balance(hiPrec.ID)
	if hipBal != 100000 {
		t.Errorf("hip balance = %d, want 100000", hipBal)
	}
}
