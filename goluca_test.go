package luca

import (
	"bytes"
	"strings"
	"testing"
)

const sampleGoluca = `2026-02-07 * Dividend payment
  Asset:Cash → Equity:Capital "Dividend" 200.00 GBP

2026-02-01 * Payroll
  +Income:Salary → Asset:Bank "net salary" 4000.00 GBP
  +Income:Salary → Expense:Tax "income tax" 1000.00 GBP

2026-03-01 ! Pending invoice
  Asset:Bank → Expense:Food 15.50 GBP
`

func TestParseGoluca(t *testing.T) {
	gf, err := ParseGoluca(strings.NewReader(sampleGoluca))
	if err != nil {
		t.Fatalf("ParseGoluca: %v", err)
	}
	if len(gf.Transactions) != 3 {
		t.Fatalf("got %d transactions, want 3", len(gf.Transactions))
	}

	// Transaction 1: single movement with payee
	txn := gf.Transactions[0]
	if txn.Date.Format("2006-01-02") != "2026-02-07" {
		t.Errorf("txn[0].Date = %s", txn.Date.Format("2006-01-02"))
	}
	if txn.Flag != '*' {
		t.Errorf("txn[0].Flag = %c, want *", txn.Flag)
	}
	if txn.Payee != "Dividend payment" {
		t.Errorf("txn[0].Payee = %q", txn.Payee)
	}
	if len(txn.Movements) != 1 {
		t.Fatalf("txn[0] has %d movements, want 1", len(txn.Movements))
	}
	m := txn.Movements[0]
	if m.From != "Asset:Cash" || m.To != "Equity:Capital" {
		t.Errorf("movement from=%q to=%q", m.From, m.To)
	}
	if m.Description != "Dividend" {
		t.Errorf("description = %q", m.Description)
	}
	if m.Amount != "200.00" {
		t.Errorf("amount = %q", m.Amount)
	}
	if m.Commodity != "GBP" {
		t.Errorf("commodity = %q", m.Commodity)
	}
	if m.Linked {
		t.Error("movement should not be linked")
	}

	// Transaction 2: linked movements
	txn2 := gf.Transactions[1]
	if len(txn2.Movements) != 2 {
		t.Fatalf("txn[1] has %d movements, want 2", len(txn2.Movements))
	}
	if !txn2.Movements[0].Linked || !txn2.Movements[1].Linked {
		t.Error("linked movements should have Linked=true")
	}
	if txn2.Movements[0].Amount != "4000.00" {
		t.Errorf("linked[0].Amount = %q", txn2.Movements[0].Amount)
	}

	// Transaction 3: pending
	txn3 := gf.Transactions[2]
	if txn3.Flag != '!' {
		t.Errorf("txn[2].Flag = %c, want !", txn3.Flag)
	}
	if txn3.Movements[0].Amount != "15.50" {
		t.Errorf("pending amount = %q", txn3.Movements[0].Amount)
	}
}

func TestParseArrowVariants(t *testing.T) {
	for _, arrow := range []string{"->", "//", "→", ">"} {
		input := "2026-01-01 *\n  Asset:Cash " + arrow + " Expense:Food 10.00 GBP\n"
		gf, err := ParseGoluca(strings.NewReader(input))
		if err != nil {
			t.Fatalf("arrow %q: %v", arrow, err)
		}
		if len(gf.Transactions) != 1 {
			t.Fatalf("arrow %q: got %d transactions", arrow, len(gf.Transactions))
		}
		m := gf.Transactions[0].Movements[0]
		if m.From != "Asset:Cash" || m.To != "Expense:Food" {
			t.Errorf("arrow %q: from=%q to=%q", arrow, m.From, m.To)
		}
	}
}

func TestParseComments(t *testing.T) {
	input := "# This is a comment\n2026-01-01 *\n  Asset:Cash → Expense:Food 10.00 GBP\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse with comment: %v", err)
	}
	if len(gf.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(gf.Transactions))
	}
}

func TestWriteToRoundTrip(t *testing.T) {
	gf, err := ParseGoluca(strings.NewReader(sampleGoluca))
	if err != nil {
		t.Fatalf("ParseGoluca: %v", err)
	}

	var buf bytes.Buffer
	_, err = gf.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	// Re-parse the output
	gf2, err := ParseGoluca(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	if len(gf2.Transactions) != len(gf.Transactions) {
		t.Fatalf("round-trip: got %d transactions, want %d", len(gf2.Transactions), len(gf.Transactions))
	}
	for i, txn := range gf.Transactions {
		txn2 := gf2.Transactions[i]
		if txn.Date != txn2.Date {
			t.Errorf("txn[%d] date mismatch", i)
		}
		if txn.Flag != txn2.Flag {
			t.Errorf("txn[%d] flag mismatch", i)
		}
		if len(txn.Movements) != len(txn2.Movements) {
			t.Errorf("txn[%d] movement count mismatch", i)
			continue
		}
		for j, m := range txn.Movements {
			m2 := txn2.Movements[j]
			if m.From != m2.From || m.To != m2.To {
				t.Errorf("txn[%d].mov[%d] path mismatch", i, j)
			}
			if m.Commodity != m2.Commodity {
				t.Errorf("txn[%d].mov[%d] commodity mismatch", i, j)
			}
		}
	}
}

func TestParseNoPayee(t *testing.T) {
	input := "2026-01-01 *\n  Asset:Cash → Expense:Food 10.00 GBP\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if gf.Transactions[0].Payee != "" {
		t.Errorf("payee = %q, want empty", gf.Transactions[0].Payee)
	}
}

func TestInferExponent(t *testing.T) {
	tests := []struct {
		amount string
		want   int
	}{
		{"200.00", -2},
		{"15.50", -2},
		{"1,000.000", -3},
		{"100", 0},
	}
	for _, tt := range tests {
		got := inferExponent(tt.amount)
		if got != tt.want {
			t.Errorf("inferExponent(%q) = %d, want %d", tt.amount, got, tt.want)
		}
	}
}
