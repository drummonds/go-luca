package luca

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	_ "github.com/drummonds/go-postgres"
)

func TestDailyInterestCalculation(t *testing.T) {
	l := newTestLedger(t)
	l.EnsureInterestAccounts()

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	// 3.65% annual rate → 0.01% daily → 0.10 on 1000.00
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.0365)

	day1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l.RecordMovement(equity.ID, savings.ID, 100000, CodeBookTransfer, day1, "Deposit") // 1000.00

	result, err := l.CalculateDailyInterest(savings.ID, day1)
	if err != nil {
		t.Fatalf("CalculateDailyInterest: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.OpeningBalance != 100000 {
		t.Errorf("OpeningBalance = %d, want 100000", result.OpeningBalance)
	}

	// Expected: 1000.00 * 0.0365 / 365 = 0.10 → 10 at exponent -2
	balDec := decimal.New(100000, -2) // 1000.00
	rate := decimal.NewFromFloat(0.0365)
	dailyRate := rate.Div(decimal.NewFromInt(365))
	expectedDec := balDec.Mul(dailyRate)
	expectedInterest := DecimalToInt(expectedDec, -2)

	if result.InterestAmount != expectedInterest {
		t.Errorf("InterestAmount = %d, want %d", result.InterestAmount, expectedInterest)
	}

	// Verify balance increased
	bal, _ := l.Balance(savings.ID)
	if bal != 100000+expectedInterest {
		t.Errorf("balance after interest = %d, want %d", bal, 100000+expectedInterest)
	}
}

func TestInterestCompounding(t *testing.T) {
	l := newTestLedger(t)
	l.EnsureInterestAccounts()

	// 10% rate on 100000.00 at -2 exponent.
	// Daily interest ≈ 100000*0.10/365 = 27.397 → 2739 at -2.
	// Day 2 interest on 100027.39 → 2740. Compounding visible.
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.10)

	day0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l.RecordMovement(equity.ID, savings.ID, 10000000, CodeBookTransfer, day0, "Deposit") // 100000.00

	// Run interest for 30 days
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC)
	results, err := l.RunInterestForPeriod(from, to)
	if err != nil {
		t.Fatalf("RunInterestForPeriod: %v", err)
	}
	if len(results) != 30 {
		t.Fatalf("got %d results, want 30", len(results))
	}

	// Day 1 interest
	day1Interest := results[0].InterestAmount

	// After 30 days, compound balance should exceed simple interest
	bal, _ := l.Balance(savings.ID)
	simpleInterest := Amount(10000000) + 30*day1Interest
	if bal <= simpleInterest {
		t.Errorf("balance %d should exceed simple interest %d due to compounding", bal, simpleInterest)
	}

	// Each day's interest should be based on the previous day's closing balance
	if results[1].OpeningBalance <= results[0].OpeningBalance {
		t.Error("day 2 opening balance should exceed day 1 (compounding)")
	}
	if results[1].InterestAmount <= results[0].InterestAmount {
		t.Errorf("day 2 interest %d should exceed day 1 interest %d (compounding)",
			results[1].InterestAmount, results[0].InterestAmount)
	}
}

func TestZeroRateNoInterest(t *testing.T) {
	l := newTestLedger(t)
	l.EnsureInterestAccounts()

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0) // zero rate

	day1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l.RecordMovement(equity.ID, cash.ID, 100000, CodeBookTransfer, day1, "Deposit") // 1000.00

	result, err := l.CalculateDailyInterest(cash.ID, day1)
	if err != nil {
		t.Fatalf("CalculateDailyInterest: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for zero rate account")
	}
}

func TestRunDailyInterestMultipleAccounts(t *testing.T) {
	l := newTestLedger(t)
	l.EnsureInterestAccounts()

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	s1, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.05)
	s2, _ := l.CreateAccount("Liability:Savings:0002", "GBP", -2, 0.03)

	day1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l.RecordMovement(equity.ID, s1.ID, 1000000, CodeBookTransfer, day1, "Deposit 1") // 10000.00
	l.RecordMovement(equity.ID, s2.ID, 500000, CodeBookTransfer, day1, "Deposit 2")  // 5000.00

	results, err := l.RunDailyInterest(day1)
	if err != nil {
		t.Fatalf("RunDailyInterest: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	// s1: 10000.00 * 0.05/365 → compute expected via decimal
	bal1Dec := decimal.New(1000000, -2)
	rate1 := decimal.NewFromFloat(0.05)
	expected1 := DecimalToInt(bal1Dec.Mul(rate1.Div(decimal.NewFromInt(365))), -2)
	if results[0].InterestAmount != expected1 {
		t.Errorf("s1 interest = %d, want %d", results[0].InterestAmount, expected1)
	}

	// s2: 5000.00 * 0.03/365
	bal2Dec := decimal.New(500000, -2)
	rate2 := decimal.NewFromFloat(0.03)
	expected2 := DecimalToInt(bal2Dec.Mul(rate2.Div(decimal.NewFromInt(365))), -2)
	if results[1].InterestAmount != expected2 {
		t.Errorf("s2 interest = %d, want %d", results[1].InterestAmount, expected2)
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
			l.RecordMovement(equity.ID, savings.ID, 100000, CodeBookTransfer, depositTime, "deposit")

			// Query balance at end of June 15 in query timezone
			endOfDay := time.Date(2026, 6, 15, 23, 59, 59, 999999999, tt.queryTZ)

			bal, err := l.BalanceAt(savings.ID, endOfDay)
			if err != nil {
				t.Fatalf("BalanceAt: %v", err)
			}
			if bal != 100000 {
				var storedTime string
				l.db.QueryRow("SELECT value_time FROM movements LIMIT 1").Scan(&storedTime)
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

// TestIssue1_InterestCrossTimezone is the end-to-end reproduction of issue #1:
// CalculateDailyInterest returns 0 when movements were recorded in a non-UTC
// timezone and interest is computed in UTC (or vice versa), and the timezone
// offset causes the date component of the RFC3339 string to differ.
func TestIssue1_InterestCrossTimezone(t *testing.T) {
	bst := time.FixedZone("BST", 1*60*60) // UTC+1
	l := newTestLedger(t)
	l.EnsureInterestAccounts()

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	// £100,000 at 3.65% annual = ~£10/day = 1000 pence/day
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.0365)

	// Deposit at 00:30 BST on June 16 = 23:30 UTC on June 15.
	// This simulates a real-world app recording time.Now() in BST near midnight.
	depositTime := time.Date(2026, 6, 16, 0, 30, 0, 0, bst)
	_, err := l.RecordMovement(equity.ID, savings.ID, 10_000_000, CodeBookTransfer, depositTime, "Deposit £100,000")
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}

	// Interest batch job uses UTC dates — common in server environments.
	interestDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	result, err := l.CalculateDailyInterest(savings.ID, interestDate)
	if err != nil {
		t.Fatalf("CalculateDailyInterest: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.InterestAmount == 0 {
		var storedTime string
		l.db.QueryRow("SELECT value_time FROM movements LIMIT 1").Scan(&storedTime)
		t.Errorf("InterestAmount = 0, want ~1000 (issue #1: returns 0 on ncruces backend)\n"+
			"  stored value_time: %q (BST midnight = UTC 23:30 June 15)\n"+
			"  endOfDay query:    %q",
			storedTime,
			time.Date(2026, 6, 15, 23, 59, 59, 999999999, time.UTC).Format(time.RFC3339Nano))
	}
}

func TestRunDailyInterestNoInterestBearingAccounts(t *testing.T) {
	l := newTestLedger(t)
	l.EnsureInterestAccounts()

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)

	day1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l.RecordMovement(equity.ID, cash.ID, 100000, CodeBookTransfer, day1, "Deposit") // 1000.00

	results, err := l.RunDailyInterest(day1)
	if err != nil {
		t.Fatalf("RunDailyInterest: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0 for no interest-bearing accounts", len(results))
	}
}
