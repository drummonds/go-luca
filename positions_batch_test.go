package luca

import (
	"testing"
	"time"
)

func TestPositionsListsEveryAccountsLatestOnOrBeforeTheDay(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	a, _ := l.CreateAccount("Liability:Savings:a", "GBP", -2, 0)
	b, _ := l.CreateAccount("Liability:Savings:b", "GBP", -2, 0)
	d1 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	d2, d3 := d1.AddDate(0, 0, 1), d1.AddDate(0, 0, 2)

	for _, m := range []struct {
		to  string
		day time.Time
		amt Amount
	}{{a.ID, d1, 100}, {b.ID, d1, 200}, {a.ID, d3, 1}} {
		if _, err := l.RecordMovementWithProjections(equity.ID, m.to, m.amt, CodeBookTransfer, m.day.Add(10*time.Hour), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.Project(b.ID, d2, Fraction{Num: 7, Den: 3650000}); err != nil {
		t.Fatal(err)
	}

	got, err := l.Positions(d2)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Position{}
	for _, p := range got {
		byID[p.AccountID] = p
	}
	if len(byID) != 3 {
		t.Fatalf("positions = %+v, want equity, a and b", got)
	}
	if p := byID[a.ID]; !p.Day.Equal(d1) || p.Balance != 100 {
		t.Errorf("a = %+v, want day 1 at 100 (day 3 is after the asked day)", p)
	}
	if p := byID[b.ID]; !p.Day.Equal(d2) || p.Balance != 200 || p.Accrued != (Fraction{Num: 7, Den: 3650000}) {
		t.Errorf("b = %+v, want day 2 at 200 with its accrual", p)
	}
	if p := byID[equity.ID]; !p.Day.Equal(d1) || p.Balance != -300 {
		t.Errorf("equity = %+v, want day 1 at -300", p)
	}
	if _, err := NewMemLedger().Positions(d1); err != ErrNotImplemented {
		t.Errorf("MemLedger.Positions: %v", err)
	}
}

// A movement that opens a new day's position carries the previous
// position's accrual forward, so a process restarting mid-day reads the
// accrual as last projected rather than zero.
func TestMovementOnANewDayCarriesTheAccrualForward(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	a, _ := l.CreateAccount("Liability:Savings:a", "GBP", -2, 0)
	d1 := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	d2 := d1.AddDate(0, 0, 1)
	accrued := Fraction{Num: 12345, Den: 3650000}
	if _, err := l.Project(a.ID, d1, accrued); err != nil {
		t.Fatal(err)
	}
	if _, err := l.RecordMovementWithProjections(equity.ID, a.ID, 500, CodeBookTransfer, d2.Add(9*time.Hour), "deposit"); err != nil {
		t.Fatal(err)
	}
	p := mustPosition(t, l, a.ID, d2)
	if !p.Day.Equal(d2) || p.Balance != 500 || p.Accrued != accrued {
		t.Errorf("day 2 = %+v, want balance 500 with day 1's accrual %+v", p, accrued)
	}
}
