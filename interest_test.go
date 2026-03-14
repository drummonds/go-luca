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
	l.RecordMovement(equity.ID, savings.ID, 100000, day1, "Deposit") // 1000.00

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
	l.RecordMovement(equity.ID, savings.ID, 10000000, day0, "Deposit") // 100000.00

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
	l.RecordMovement(equity.ID, cash.ID, 100000, day1, "Deposit") // 1000.00

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
	l.RecordMovement(equity.ID, s1.ID, 1000000, day1, "Deposit 1") // 10000.00
	l.RecordMovement(equity.ID, s2.ID, 500000, day1, "Deposit 2")  // 5000.00

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

func TestRunDailyInterestNoInterestBearingAccounts(t *testing.T) {
	l := newTestLedger(t)
	l.EnsureInterestAccounts()

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)

	day1 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l.RecordMovement(equity.ID, cash.ID, 100000, day1, "Deposit") // 1000.00

	results, err := l.RunDailyInterest(day1)
	if err != nil {
		t.Fatalf("RunDailyInterest: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0 for no interest-bearing accounts", len(results))
	}
}
