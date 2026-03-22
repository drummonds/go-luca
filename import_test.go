package luca

import (
	"bytes"
	"strings"
	"testing"

	_ "github.com/drummonds/go-postgres"
)

func TestImportAutoCreate(t *testing.T) {
	l := newTestLedger(t)

	input := `2026-02-07 * Dividend payment
  Asset:Cash → Equity:Capital "Dividend" 200.00 GBP
`
	err := l.ImportString(input, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	// Verify accounts were created
	cash, err := l.GetAccount("Asset:Cash")
	if err != nil || cash == nil {
		t.Fatal("Asset:Cash not created")
	}
	if cash.Exponent != -2 {
		t.Errorf("cash exponent = %d, want -2", cash.Exponent)
	}
	if cash.Commodity != "GBP" {
		t.Errorf("cash commodity = %q, want GBP", cash.Commodity)
	}

	capital, err := l.GetAccount("Equity:Capital")
	if err != nil || capital == nil {
		t.Fatal("Equity:Capital not created")
	}

	// Verify balance
	bal, err := l.Balance(capital.ID)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal != 20000 { // 200.00 in pence
		t.Errorf("balance = %d, want 20000", bal)
	}
}

func TestImportNoAutoCreate(t *testing.T) {
	l := newTestLedger(t)

	input := `2026-02-07 *
  Asset:Cash → Equity:Capital 100.00 GBP
`
	opts := &ImportOptions{AutoCreateAccounts: false}
	err := l.ImportString(input, opts)
	if err == nil {
		t.Fatal("expected error for missing accounts")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestImportLinkedMovements(t *testing.T) {
	l := newTestLedger(t)

	input := `2026-02-01 * Payroll
  Income:Salary → Asset:Bank "net salary" 4000.00 GBP
  +Income:Salary → Expense:Tax "income tax" 1000.00 GBP
`
	err := l.ImportString(input, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	bank, _ := l.GetAccount("Asset:Bank")
	tax, _ := l.GetAccount("Expense:Tax")

	bankBal, _ := l.Balance(bank.ID)
	if bankBal != 400000 {
		t.Errorf("bank balance = %d, want 400000", bankBal)
	}
	taxBal, _ := l.Balance(tax.ID)
	if taxBal != 100000 {
		t.Errorf("tax balance = %d, want 100000", taxBal)
	}
}

func TestImportPending(t *testing.T) {
	l := newTestLedger(t)

	input := `2026-03-01 ! Pending invoice
  Asset:Bank → Expense:Food 15.50 GBP
`
	err := l.ImportString(input, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	food, _ := l.GetAccount("Expense:Food")
	bal, _ := l.Balance(food.ID)
	if bal != 1550 {
		t.Errorf("food balance = %d, want 1550", bal)
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	l1 := newTestLedger(t)

	input := `2026-02-01 * Transfer
  Asset:Cash → Asset:Bank 100.00 GBP

2026-02-02 * Payroll
  Income:Salary → Asset:Bank "net salary" 4000.00 GBP
  +Income:Salary → Expense:Tax "income tax" 1000.00 GBP
`
	err := l1.ImportString(input, nil)
	if err != nil {
		t.Fatalf("Import 1: %v", err)
	}

	// Export from l1
	var buf1 bytes.Buffer
	if err := l1.Export(&buf1); err != nil {
		t.Fatalf("Export 1: %v", err)
	}

	// Import into fresh ledger
	l2 := newTestLedger(t)
	err = l2.ImportString(buf1.String(), nil)
	if err != nil {
		t.Fatalf("Import 2: %v", err)
	}

	// Export from l2
	var buf2 bytes.Buffer
	if err := l2.Export(&buf2); err != nil {
		t.Fatalf("Export 2: %v", err)
	}

	// Compare
	if buf1.String() != buf2.String() {
		t.Errorf("round-trip mismatch:\n--- export 1 ---\n%s\n--- export 2 ---\n%s", buf1.String(), buf2.String())
	}
}
