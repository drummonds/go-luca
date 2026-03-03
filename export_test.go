package luca

import (
	"bytes"
	"strings"
	"testing"
	"time"

	_ "github.com/drummonds/go-postgres"
)

func TestExport(t *testing.T) {
	l := newTestLedger(t)

	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	bank, _ := l.CreateAccount("Asset:Bank", "GBP", -2, 0)
	salary, _ := l.CreateAccount("Income:Salary", "GBP", -2, 0)

	date := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	// Single movement
	l.RecordMovement(cash.ID, bank.ID, 10000, date, "Transfer")

	// Linked movements
	l.RecordLinkedMovements([]MovementInput{
		{FromAccountID: salary.ID, ToAccountID: bank.ID, Amount: 400000, Description: "net salary"},
		{FromAccountID: salary.ID, ToAccountID: cash.ID, Amount: 50000, Description: "cash advance"},
	}, date.AddDate(0, 0, 1))

	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("Export: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Asset:Cash") {
		t.Error("export missing Asset:Cash")
	}
	if !strings.Contains(output, "Asset:Bank") {
		t.Error("export missing Asset:Bank")
	}
	if !strings.Contains(output, "100") {
		t.Error("export missing amount 100")
	}

	// Verify linked movements have + prefix
	if !strings.Contains(output, "+") {
		t.Error("export missing linked prefix +")
	}

	// Verify it re-parses
	gf, err := ParseGoluca(strings.NewReader(output))
	if err != nil {
		t.Fatalf("re-parse exported: %v", err)
	}
	if len(gf.Transactions) != 2 {
		t.Errorf("got %d transactions, want 2", len(gf.Transactions))
	}
}

func TestExportEmpty(t *testing.T) {
	l := newTestLedger(t)
	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}
