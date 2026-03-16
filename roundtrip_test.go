package luca

import (
	"bytes"
	"strings"
	"testing"

	_ "github.com/drummonds/go-postgres"
)

// roundTrip is a test helper that imports goluca text, exports it, re-parses,
// and returns the re-parsed GolucaFile. Fails the test on any error.
func roundTrip(t *testing.T, input string) *GolucaFile {
	t.Helper()
	l := newTestLedger(t)
	if err := l.ImportString(input, nil); err != nil {
		t.Fatalf("import: %v", err)
	}
	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	gf, err := ParseGoluca(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	return gf
}

// These tests document what survives and what's lost when goluca text
// goes through the DB (import → DB → export).

func TestDBRoundTripBasicTransaction(t *testing.T) {
	l := newTestLedger(t)

	input := `2026-02-07 * Dividend payment
  Asset:Cash → Equity:Capital "Dividend" 200.00 GBP
`
	if err := l.ImportString(input, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("export: %v", err)
	}

	gf, err := ParseGoluca(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(gf.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(gf.Transactions))
	}
	txn := gf.Transactions[0]
	if txn.DateTime.Date != "2026-02-07" {
		t.Errorf("date = %q, want 2026-02-07", txn.DateTime.Date)
	}
	m := txn.Movements[0]
	if m.From != "Asset:Cash" || m.To != "Equity:Capital" {
		t.Errorf("paths: from=%q to=%q", m.From, m.To)
	}
	if m.Amount != "200" {
		t.Errorf("amount = %q, want 200", m.Amount)
	}
}

func TestDBRoundTripLinkedMovements(t *testing.T) {
	l := newTestLedger(t)

	input := `2026-02-01 * Payroll
  Income:Salary → Asset:Bank "net salary" 4000.00 GBP
  +Income:Salary → Expense:Tax "income tax" 1000.00 GBP
`
	if err := l.ImportString(input, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("export: %v", err)
	}

	gf, err := ParseGoluca(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(gf.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(gf.Transactions))
	}
	if len(gf.Transactions[0].Movements) != 2 {
		t.Errorf("got %d movements, want 2", len(gf.Transactions[0].Movements))
	}
	// Linked movements survive through batch_id grouping
	if !gf.Transactions[0].Movements[0].Linked {
		t.Error("expected movements to be linked after round-trip")
	}
}

// TestDBRoundTripTimestampLosesTime documents that DB stores value_time as
// TIMESTAMP which, through pglike, stores as TEXT. Non-midnight times
// cause the exported date to include a time component.
func TestDBRoundTripTimestampLosesTime(t *testing.T) {
	l := newTestLedger(t)

	// Import with date-only (midnight) — should round-trip cleanly
	input := `2026-02-07 * Transfer
  Asset:Cash → Asset:Bank 100.00 GBP
`
	if err := l.ImportString(input, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("export: %v", err)
	}

	gf, err := ParseGoluca(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	// Date-only input should come back as date-only
	if gf.Transactions[0].DateTime.Date != "2026-02-07" {
		t.Errorf("date = %q, want 2026-02-07", gf.Transactions[0].DateTime.Date)
	}
}

// TestDBRoundTripKnowledgeDateTimeLost documents that knowledge_datetime
// from the goluca file is NOT preserved through the DB. The DB sets
// knowledge_time to NOW() on insert, discarding the file's value.
func TestDBRoundTripKnowledgeDateTime(t *testing.T) {
	l := newTestLedger(t)

	input := `2026-01-15%2026-01-20 * Late booking
  Asset:CreditCard → Expense:Groceries 45.50 GBP
`
	if err := l.ImportString(input, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("export: %v", err)
	}

	gf, err := ParseGoluca(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	txn := gf.Transactions[0]
	if txn.KnowledgeDateTime == nil {
		t.Fatal("knowledge datetime not preserved through DB round-trip")
	}
	if txn.KnowledgeDateTime.Date != "2026-01-20" {
		t.Errorf("knowledge date = %q, want 2026-01-20", txn.KnowledgeDateTime.Date)
	}
}

// TestDBRoundTripMetadata verifies transaction metadata survives DB round-trip.
func TestDBRoundTripMetadataLost(t *testing.T) {

	l := newTestLedger(t)

	input := `2026-01-15 * Purchase
  Asset:Bank → Expense:Food 45.50 GBP
  receipt: scan-001.pdf
  category: groceries
`
	if err := l.ImportString(input, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("export: %v", err)
	}

	gf, err := ParseGoluca(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	txn := gf.Transactions[0]
	if txn.Metadata == nil || txn.Metadata["receipt"] != "scan-001.pdf" {
		t.Error("metadata not preserved through DB round-trip")
	}
}

// TestDBRoundTripDirectives verifies directives survive DB round-trip.
func TestDBRoundTripDirectivesLost(t *testing.T) {

	l := newTestLedger(t)

	input := `option operating-currency GBP

2024-01-01 commodity GBP
  name: British Pound Sterling

2024-01-01 open Asset:Bank:Current GBP
alias Current Asset:Bank:Current

2024-01-15 data interest:base-rate 5.25

2024-01-18 * Online purchase
  Asset:CreditCard → Expense:Groceries "Delivery order" 32.00 GBP
`
	if err := l.ImportString(input, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("export: %v", err)
	}

	gf, err := ParseGoluca(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(gf.Options) != 1 {
		t.Error("options not preserved through DB round-trip")
	}
	if len(gf.Commodities) != 1 {
		t.Error("commodities not preserved through DB round-trip")
	}
	if len(gf.Opens) != 1 {
		t.Error("opens not preserved through DB round-trip")
	}
	if len(gf.Aliases) != 1 {
		t.Error("aliases not preserved through DB round-trip")
	}
	if len(gf.DataPoints) != 1 {
		t.Error("data points not preserved through DB round-trip")
	}
}

// TestDBRoundTripPeriodAnchorLost documents that period anchors
// have no DB representation — they're resolved to timestamps on import.
func TestDBRoundTripPeriodAnchor(t *testing.T) {
	l := newTestLedger(t)

	input := `2024-01-15T23:59:59$ * End of day close
  Asset:Bank → Equity:Close 0.00 GBP
`
	if err := l.ImportString(input, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	var buf bytes.Buffer
	if err := l.Export(&buf); err != nil {
		t.Fatalf("export: %v", err)
	}

	gf, err := ParseGoluca(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	txn := gf.Transactions[0]
	if txn.DateTime.PeriodAnchor != "$" {
		t.Error("period anchor not preserved through DB round-trip")
	}
}

// --- Comprehensive round-trip test cases ---

func TestRoundTripPendingTransaction(t *testing.T) {
	gf := roundTrip(t, `2026-03-01 ! Pending invoice
  Asset:Bank → Expense:Food 15.50 GBP
`)
	if len(gf.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(gf.Transactions))
	}
	if gf.Transactions[0].Flag != '!' {
		t.Errorf("flag = %c, want !", gf.Transactions[0].Flag)
	}
}

func TestRoundTripZeroAmount(t *testing.T) {
	gf := roundTrip(t, `2026-03-01 * Zero transfer
  Asset:Bank → Equity:Close 0.00 GBP
`)
	if len(gf.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(gf.Transactions))
	}
	if gf.Transactions[0].Movements[0].Amount != "0" {
		t.Errorf("amount = %q, want 0", gf.Transactions[0].Movements[0].Amount)
	}
}

func TestRoundTripNoPayee(t *testing.T) {
	gf := roundTrip(t, `2026-03-01 *
  Asset:Bank → Expense:Food 25.00 GBP
`)
	if len(gf.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(gf.Transactions))
	}
	// Single movement with no payee and no description — payee comes from description
	// which is empty, so payee should be empty
}

func TestRoundTripMultipleTransactions(t *testing.T) {
	gf := roundTrip(t, `2026-01-01 * First
  Asset:Cash → Equity:Capital 100.00 GBP

2026-01-02 * Second
  Equity:Capital → Asset:Cash 50.00 GBP

2026-01-03 * Third
  Asset:Cash → Expense:Food 25.00 GBP
`)
	if len(gf.Transactions) != 3 {
		t.Fatalf("got %d transactions, want 3", len(gf.Transactions))
	}
	dates := []string{"2026-01-01", "2026-01-02", "2026-01-03"}
	for i, want := range dates {
		if gf.Transactions[i].DateTime.Date != want {
			t.Errorf("txn %d date = %q, want %q", i, gf.Transactions[i].DateTime.Date, want)
		}
	}
}

func TestRoundTripMixedDirectivesAndTransactions(t *testing.T) {
	gf := roundTrip(t, `option operating-currency GBP

2024-01-01 commodity GBP
  name: British Pound Sterling

2024-01-01 open Asset:Bank GBP
alias Bank Asset:Bank

2024-06-15 data rates:base 5.25

2024-01-15 * Deposit
  Equity:Capital → Asset:Bank 1000.00 GBP
`)
	if len(gf.Options) != 1 {
		t.Errorf("options = %d, want 1", len(gf.Options))
	}
	if len(gf.Commodities) != 1 {
		t.Errorf("commodities = %d, want 1", len(gf.Commodities))
	}
	if len(gf.Opens) != 1 {
		t.Errorf("opens = %d, want 1", len(gf.Opens))
	}
	if len(gf.Aliases) != 1 {
		t.Errorf("aliases = %d, want 1", len(gf.Aliases))
	}
	if len(gf.DataPoints) != 1 {
		t.Errorf("data points = %d, want 1", len(gf.DataPoints))
	}
	if len(gf.Transactions) != 1 {
		t.Errorf("transactions = %d, want 1", len(gf.Transactions))
	}
}

func TestRoundTripDiffTransactions(t *testing.T) {
	// Verify DiffTransactions detects no changes on a clean round-trip
	input := `2026-01-15 * Purchase
  Asset:Bank → Expense:Food 45.50 GBP
`
	gfOrig, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse original: %v", err)
	}

	gfRT := roundTrip(t, input)

	if len(gfOrig.Transactions) != 1 || len(gfRT.Transactions) != 1 {
		t.Fatal("expected 1 transaction each")
	}

	diff := DiffTransactions(gfOrig.Transactions[0], gfRT.Transactions[0])
	if diff.DateTimeChanged {
		t.Error("datetime should not change on round-trip")
	}
	if len(diff.MovementsAdded) > 0 || len(diff.MovementsRemoved) > 0 {
		t.Error("movements should not change on round-trip")
	}
}

func TestRoundTripDataPointTypes(t *testing.T) {
	gf := roundTrip(t, `2024-01-15 data rate:annual 5.25
2024-01-16 data flag:active true
2024-01-17 data label:name some-text
`)
	if len(gf.DataPoints) != 3 {
		t.Fatalf("got %d data points, want 3", len(gf.DataPoints))
	}
	// Values should survive (stored as text)
	values := map[string]string{
		"rate:annual": "5.25",
		"flag:active": "true",
		"label:name":  "some-text",
	}
	for _, dp := range gf.DataPoints {
		want, ok := values[dp.ParamName]
		if !ok {
			t.Errorf("unexpected data point %q", dp.ParamName)
			continue
		}
		if dp.ParamValue != want {
			t.Errorf("data point %q = %q, want %q", dp.ParamName, dp.ParamValue, want)
		}
	}
}

func TestRoundTripTransactionMetadata(t *testing.T) {
	gf := roundTrip(t, `2026-01-15 * Purchase
  Asset:Bank → Expense:Food 45.50 GBP
  receipt: scan-001.pdf
  category: groceries
  notes: weekly shop
`)
	if len(gf.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(gf.Transactions))
	}
	meta := gf.Transactions[0].Metadata
	if meta == nil {
		t.Fatal("metadata lost on round-trip")
	}
	for _, key := range []string{"receipt", "category", "notes"} {
		if _, ok := meta[key]; !ok {
			t.Errorf("metadata key %q missing", key)
		}
	}
	if meta["receipt"] != "scan-001.pdf" {
		t.Errorf("receipt = %q, want scan-001.pdf", meta["receipt"])
	}
}
