package luca

import (
	"bytes"
	"strings"
	"testing"
)

const sampleGoluca = `2026-02-07 * Dividend payment
  Asset:Cash → Equity:Capital "Dividend" 200.00 GBP

2026-02-01 * Payroll
  Income:Salary → Asset:Bank "net salary" 4000.00 GBP
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
	if txn.DateTime.Date != "2026-02-07" {
		t.Errorf("txn[0].DateTime.Date = %s", txn.DateTime.Date)
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
		if m.Arrow != arrow {
			t.Errorf("arrow %q: Arrow=%q", arrow, m.Arrow)
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

// --- DateTime variant tests ---

func TestParseDateTimeVariants(t *testing.T) {
	tests := []struct {
		name string
		input string
		want DateTime
	}{
		{
			"date only",
			"2026-02-07 *\n  Asset:Cash -> Expense:Food 10.00 GBP\n",
			DateTime{Date: "2026-02-07"},
		},
		{
			"full UTC",
			"2026-02-07T14:30:00Z *\n  Asset:Cash -> Expense:Food 10.00 GBP\n",
			DateTime{Date: "2026-02-07", Time: "14:30:00", Timezone: "Z"},
		},
		{
			"tz offset positive",
			"2026-02-07T14:30:00+01:00 *\n  Asset:Cash -> Expense:Food 10.00 GBP\n",
			DateTime{Date: "2026-02-07", Time: "14:30:00", Timezone: "+01:00"},
		},
		{
			"milliseconds",
			"2026-02-07T14:30:00.123Z *\n  Asset:Cash -> Expense:Food 10.00 GBP\n",
			DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123", Timezone: "Z"},
		},
		{
			"microseconds with tz",
			"2026-02-07T14:30:00.123456+05:30 *\n  Asset:Cash -> Expense:Food 10.00 GBP\n",
			DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123456", Timezone: "+05:30"},
		},
		{
			"nanoseconds",
			"2026-02-07T14:30:00.123456789Z *\n  Asset:Cash -> Expense:Food 10.00 GBP\n",
			DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123456789", Timezone: "Z"},
		},
		{
			"period end with time",
			"2024-01-15T23:59:59$ * End of day close\n  Asset:Bank -> Equity:Close 0.00 GBP\n",
			DateTime{Date: "2024-01-15", Time: "23:59:59", PeriodAnchor: "$"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gf, err := ParseGoluca(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(gf.Transactions) != 1 {
				t.Fatalf("got %d transactions, want 1", len(gf.Transactions))
			}
			got := gf.Transactions[0].DateTime
			if got != tt.want {
				t.Errorf("DateTime = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// --- Knowledge datetime tests ---

func TestParseKnowledgeDateTime(t *testing.T) {
	input := "2026-01-15%2026-01-20 * Late booking\n  Asset:CreditCard -> Expense:Groceries 45.50 GBP\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(gf.Transactions) != 1 {
		t.Fatalf("got %d transactions, want 1", len(gf.Transactions))
	}
	txn := gf.Transactions[0]
	if txn.DateTime.Date != "2026-01-15" {
		t.Errorf("DateTime.Date = %q, want 2026-01-15", txn.DateTime.Date)
	}
	if txn.KnowledgeDateTime == nil {
		t.Fatal("expected knowledge datetime, got nil")
	}
	if txn.KnowledgeDateTime.Date != "2026-01-20" {
		t.Errorf("KnowledgeDateTime.Date = %q, want 2026-01-20", txn.KnowledgeDateTime.Date)
	}
}

func TestParseKnowledgeDateTimeFull(t *testing.T) {
	input := "2026-01-15T10:00:00Z%2026-01-20T09:00:00+01:00 * Late booking\n  Asset:Bank -> Expense:Food 20.00 GBP\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	txn := gf.Transactions[0]
	if txn.DateTime.Time != "10:00:00" || txn.DateTime.Timezone != "Z" {
		t.Errorf("value datetime = %+v", txn.DateTime)
	}
	if txn.KnowledgeDateTime == nil {
		t.Fatal("expected knowledge datetime")
	}
	if txn.KnowledgeDateTime.Time != "09:00:00" || txn.KnowledgeDateTime.Timezone != "+01:00" {
		t.Errorf("knowledge datetime = %+v", *txn.KnowledgeDateTime)
	}
}

// --- Directive tests ---

func TestParseOptionDirective(t *testing.T) {
	input := "option operating-currency GBP\noption require-accounts true\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(gf.Options) != 2 {
		t.Fatalf("got %d options, want 2", len(gf.Options))
	}
	if gf.Options[0].Key != "operating-currency" || gf.Options[0].Value != "GBP" {
		t.Errorf("option[0] = %+v", gf.Options[0])
	}
	if gf.Options[1].Key != "require-accounts" || gf.Options[1].Value != "true" {
		t.Errorf("option[1] = %+v", gf.Options[1])
	}
}

func TestParseAliasDirective(t *testing.T) {
	input := "alias Current Assets:Bank:Current\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(gf.Aliases) != 1 {
		t.Fatalf("got %d aliases, want 1", len(gf.Aliases))
	}
	if gf.Aliases[0].Name != "Current" || gf.Aliases[0].Account != "Assets:Bank:Current" {
		t.Errorf("alias = %+v", gf.Aliases[0])
	}
}

func TestParseCommodityDirective(t *testing.T) {
	input := "2024-01-01 commodity GBP\n  name: British Pound Sterling\n  precision: 2\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(gf.Commodities) != 1 {
		t.Fatalf("got %d commodities, want 1", len(gf.Commodities))
	}
	c := gf.Commodities[0]
	if c.Code != "GBP" {
		t.Errorf("Code = %q, want GBP", c.Code)
	}
	if c.DateTime == nil || c.DateTime.Date != "2024-01-01" {
		t.Errorf("DateTime = %v", c.DateTime)
	}
	if c.Metadata["name"] != "British Pound Sterling" {
		t.Errorf("Metadata[name] = %q", c.Metadata["name"])
	}
	if c.Metadata["precision"] != "2" {
		t.Errorf("Metadata[precision] = %q", c.Metadata["precision"])
	}
}

func TestParseCommodityDirectiveNoDate(t *testing.T) {
	input := "commodity GBP\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(gf.Commodities) != 1 {
		t.Fatalf("got %d commodities, want 1", len(gf.Commodities))
	}
	if gf.Commodities[0].DateTime != nil {
		t.Errorf("expected nil DateTime, got %v", gf.Commodities[0].DateTime)
	}
	if gf.Commodities[0].Code != "GBP" {
		t.Errorf("Code = %q", gf.Commodities[0].Code)
	}
}

func TestParseOpenDirective(t *testing.T) {
	input := "2024-01-01 open Assets:Bank:Current GBP\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(gf.Opens) != 1 {
		t.Fatalf("got %d opens, want 1", len(gf.Opens))
	}
	o := gf.Opens[0]
	if o.Account != "Assets:Bank:Current" {
		t.Errorf("Account = %q", o.Account)
	}
	if o.DateTime.Date != "2024-01-01" {
		t.Errorf("DateTime.Date = %q", o.DateTime.Date)
	}
	if len(o.Commodities) != 1 || o.Commodities[0] != "GBP" {
		t.Errorf("Commodities = %v", o.Commodities)
	}
}

func TestParseOpenDirectiveMultipleCommodities(t *testing.T) {
	input := "2024-01-01 open Assets:Bank:Current GBP,USD,EUR\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	o := gf.Opens[0]
	if len(o.Commodities) != 3 {
		t.Fatalf("got %d commodities, want 3", len(o.Commodities))
	}
	want := []string{"GBP", "USD", "EUR"}
	for i, w := range want {
		if o.Commodities[i] != w {
			t.Errorf("Commodities[%d] = %q, want %q", i, o.Commodities[i], w)
		}
	}
}

func TestParseOpenDirectiveWithMetadata(t *testing.T) {
	input := "2024-01-01 open Assets:Bank:Current GBP\n  bank: HSBC\n  sort-code: 12-34-56\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	o := gf.Opens[0]
	if o.Metadata["bank"] != "HSBC" {
		t.Errorf("Metadata[bank] = %q", o.Metadata["bank"])
	}
	if o.Metadata["sort-code"] != "12-34-56" {
		t.Errorf("Metadata[sort-code] = %q", o.Metadata["sort-code"])
	}
}

func TestParseCustomerDirective(t *testing.T) {
	input := "customer \"John Smith\"\n  account Assets:Receivables:JohnSmith\n  max-aggregate-balance 10000 GBP\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(gf.Customers) != 1 {
		t.Fatalf("got %d customers, want 1", len(gf.Customers))
	}
	c := gf.Customers[0]
	if c.Name != "John Smith" {
		t.Errorf("Name = %q", c.Name)
	}
	if c.Account != "Assets:Receivables:JohnSmith" {
		t.Errorf("Account = %q", c.Account)
	}
	if c.MaxBalanceAmount != "10000" {
		t.Errorf("MaxBalanceAmount = %q", c.MaxBalanceAmount)
	}
	if c.MaxBalanceCommodity != "GBP" {
		t.Errorf("MaxBalanceCommodity = %q", c.MaxBalanceCommodity)
	}
}

func TestParseCustomerDirectiveWithMetadata(t *testing.T) {
	input := "customer \"Jane Doe\"\n  account Assets:Receivables:JaneDoe\n  max-aggregate-balance 5000 GBP\n  tier: premium\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := gf.Customers[0]
	if c.Metadata["tier"] != "premium" {
		t.Errorf("Metadata[tier] = %q", c.Metadata["tier"])
	}
}

func TestParseDataPoint(t *testing.T) {
	input := "2024-01-15 data interest:base-rate 5.25\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(gf.DataPoints) != 1 {
		t.Fatalf("got %d data points, want 1", len(gf.DataPoints))
	}
	dp := gf.DataPoints[0]
	if dp.DateTime.Date != "2024-01-15" {
		t.Errorf("DateTime.Date = %q", dp.DateTime.Date)
	}
	if dp.ParamName != "interest:base-rate" {
		t.Errorf("ParamName = %q", dp.ParamName)
	}
	if dp.ParamValue != "5.25" {
		t.Errorf("ParamValue = %q", dp.ParamValue)
	}
	if dp.KnowledgeDateTime != nil {
		t.Errorf("expected nil KnowledgeDateTime, got %v", dp.KnowledgeDateTime)
	}
}

func TestParseDataPointWithKnowledge(t *testing.T) {
	input := "2024-01-15%2024-01-10 data interest:base-rate 5.25\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dp := gf.DataPoints[0]
	if dp.KnowledgeDateTime == nil {
		t.Fatal("expected knowledge datetime")
	}
	if dp.KnowledgeDateTime.Date != "2024-01-10" {
		t.Errorf("KnowledgeDateTime.Date = %q", dp.KnowledgeDateTime.Date)
	}
}

func TestParseDataPointYearOnly(t *testing.T) {
	input := "2024 data annual:budget 50000\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dp := gf.DataPoints[0]
	if dp.DateTime.Date != "2024" {
		t.Errorf("DateTime.Date = %q, want 2024", dp.DateTime.Date)
	}
	if dp.DateTime.DateGranularity() != "year" {
		t.Errorf("granularity = %q, want year", dp.DateTime.DateGranularity())
	}
}

func TestParseDataPointPeriodAnchor(t *testing.T) {
	input := "2024$ data annual:close balance\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dp := gf.DataPoints[0]
	if dp.DateTime.PeriodAnchor != "$" {
		t.Errorf("PeriodAnchor = %q, want $", dp.DateTime.PeriodAnchor)
	}
}

// --- Metadata tests ---

func TestParseMetadataOnTransaction(t *testing.T) {
	input := "2026-01-15 * Purchase\n  Asset:Bank -> Expense:Food 45.50 GBP\n  receipt: scan-001.pdf\n  category: groceries\n"
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	txn := gf.Transactions[0]
	if txn.Metadata == nil {
		t.Fatal("expected metadata, got nil")
	}
	if txn.Metadata["receipt"] != "scan-001.pdf" {
		t.Errorf("Metadata[receipt] = %q", txn.Metadata["receipt"])
	}
	if txn.Metadata["category"] != "groceries" {
		t.Errorf("Metadata[category] = %q", txn.Metadata["category"])
	}
}

// --- Mixed file test ---

func TestParseMixedFile(t *testing.T) {
	input := `option operating-currency GBP

2024-01-01 commodity GBP
  name: British Pound Sterling

2024-01-01 open Assets:Bank:Current GBP
alias Current Assets:Bank:Current

2024-01-15 data interest:base-rate 5.25

2024-01-18 * Online purchase
  Assets:CreditCard -> Expenses:Groceries "Delivery order" 32.00 GBP
`
	gf, err := ParseGoluca(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(gf.Options) != 1 {
		t.Errorf("options: got %d, want 1", len(gf.Options))
	}
	if len(gf.Commodities) != 1 {
		t.Errorf("commodities: got %d, want 1", len(gf.Commodities))
	}
	if len(gf.Opens) != 1 {
		t.Errorf("opens: got %d, want 1", len(gf.Opens))
	}
	if len(gf.Aliases) != 1 {
		t.Errorf("aliases: got %d, want 1", len(gf.Aliases))
	}
	if len(gf.DataPoints) != 1 {
		t.Errorf("data points: got %d, want 1", len(gf.DataPoints))
	}
	if len(gf.Transactions) != 1 {
		t.Errorf("transactions: got %d, want 1", len(gf.Transactions))
	}
}

// --- WriteTo tests ---

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
		if txn.DateTime != txn2.DateTime {
			t.Errorf("txn[%d] datetime mismatch: %+v vs %+v", i, txn.DateTime, txn2.DateTime)
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

func TestWriteToWithKnowledgeDateTime(t *testing.T) {
	kdt := DateTime{Date: "2026-01-20"}
	gf := &GolucaFile{
		Transactions: []Transaction{
			{
				DateTime:          DateTime{Date: "2026-01-15"},
				KnowledgeDateTime: &kdt,
				Flag:              '*',
				Payee:             "Late booking",
				Movements: []TextMovement{
					{From: "Asset:Bank", To: "Expense:Food", Amount: "20.00", Commodity: "GBP"},
				},
			},
		},
	}
	var buf bytes.Buffer
	if _, err := gf.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "2026-01-15%2026-01-20") {
		t.Errorf("output missing knowledge datetime: %s", output)
	}
}

func TestWriteToWithDirectives(t *testing.T) {
	gf := &GolucaFile{
		Options: []Option{
			{Key: "operating-currency", Value: "GBP"},
		},
		Commodities: []CommodityDef{
			{DateTime: &DateTime{Date: "2024-01-01"}, Code: "GBP", Metadata: map[string]string{"name": "British Pound Sterling"}},
		},
		Opens: []OpenDef{
			{DateTime: DateTime{Date: "2024-01-01"}, Account: "Assets:Bank:Current", Commodities: []string{"GBP"}},
		},
		Aliases: []AliasDef{
			{Name: "Current", Account: "Assets:Bank:Current"},
		},
		DataPoints: []DataPoint{
			{DateTime: DateTime{Date: "2024-01-15"}, ParamName: "interest:base-rate", ParamValue: "5.25"},
		},
		Transactions: []Transaction{
			{
				DateTime: DateTime{Date: "2024-01-18"},
				Flag:     '*',
				Payee:    "Purchase",
				Movements: []TextMovement{
					{From: "Asset:Bank", To: "Expense:Food", Amount: "10.00", Commodity: "GBP"},
				},
			},
		},
	}

	var buf bytes.Buffer
	if _, err := gf.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}

	output := buf.String()

	// Verify all directive types appear
	if !strings.Contains(output, "option operating-currency GBP") {
		t.Error("missing option directive")
	}
	if !strings.Contains(output, "commodity GBP") {
		t.Error("missing commodity directive")
	}
	if !strings.Contains(output, "open Assets:Bank:Current GBP") {
		t.Error("missing open directive")
	}
	if !strings.Contains(output, "alias Current Assets:Bank:Current") {
		t.Error("missing alias directive")
	}
	if !strings.Contains(output, "data interest:base-rate 5.25") {
		t.Error("missing data point")
	}

	// Re-parse and verify structure
	gf2, err := ParseGoluca(strings.NewReader(output))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(gf2.Options) != 1 {
		t.Errorf("round-trip options: got %d", len(gf2.Options))
	}
	if len(gf2.Commodities) != 1 {
		t.Errorf("round-trip commodities: got %d", len(gf2.Commodities))
	}
	if len(gf2.Opens) != 1 {
		t.Errorf("round-trip opens: got %d", len(gf2.Opens))
	}
	if len(gf2.Aliases) != 1 {
		t.Errorf("round-trip aliases: got %d", len(gf2.Aliases))
	}
	if len(gf2.DataPoints) != 1 {
		t.Errorf("round-trip data points: got %d", len(gf2.DataPoints))
	}
	if len(gf2.Transactions) != 1 {
		t.Errorf("round-trip transactions: got %d", len(gf2.Transactions))
	}
}

func TestWriteToPreservesArrow(t *testing.T) {
	gf := &GolucaFile{
		Transactions: []Transaction{
			{
				DateTime: DateTime{Date: "2026-01-01"},
				Flag:     '*',
				Movements: []TextMovement{
					{From: "Asset:Cash", To: "Expense:Food", Arrow: "->", Amount: "10.00", Commodity: "GBP"},
				},
			},
		},
	}
	var buf bytes.Buffer
	if _, err := gf.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !strings.Contains(buf.String(), "->") {
		t.Errorf("arrow not preserved: %s", buf.String())
	}
}

func TestWriteToWithMetadata(t *testing.T) {
	gf := &GolucaFile{
		Transactions: []Transaction{
			{
				DateTime: DateTime{Date: "2026-01-15"},
				Flag:     '*',
				Payee:    "Purchase",
				Movements: []TextMovement{
					{From: "Asset:Bank", To: "Expense:Food", Amount: "45.50", Commodity: "GBP"},
				},
				Metadata: map[string]string{"receipt": "scan-001.pdf"},
			},
		},
	}
	var buf bytes.Buffer
	if _, err := gf.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !strings.Contains(buf.String(), "receipt: scan-001.pdf") {
		t.Errorf("metadata not written: %s", buf.String())
	}
}

// --- Account path variant tests ---

func TestParseAccountPathVariants(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
	}{
		{"two-part", "Asset:Cash", "Expense:Food"},
		{"three-part", "Equity:Capital:001", "Asset:Bank"},
		{"four-part hyphenated", "Liability:InterestAccount:0000-111:Main", "Asset:Bank"},
		{"pending suffix", "Liability:InterestAccount:0000-111:Pending", "Asset:Bank"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := "2026-01-01 *\n  " + tt.from + " -> " + tt.to + " 100.00 GBP\n"
			gf, err := ParseGoluca(strings.NewReader(input))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			m := gf.Transactions[0].Movements[0]
			if m.From != tt.from {
				t.Errorf("From = %q, want %q", m.From, tt.from)
			}
			if m.To != tt.to {
				t.Errorf("To = %q, want %q", m.To, tt.to)
			}
		})
	}
}
