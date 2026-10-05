package luca

import (
	"testing"
	"time"
)

// A hot account (a cash or P&L account that every customer movement
// touches) must cost the same per movement however many it takes in a
// day: projecting a movement moves the existing positions by its amount
// rather than re-summing the day.
func TestHotAccountProjectionsStayExactUnderManyMovements(t *testing.T) {
	l := newTestLedger(t)
	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	day := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	const n = 200
	var ids []string
	for i := range n {
		a, err := l.CreateAccount("Liability:Savings:"+string(rune('a'+i%26))+string(rune('a'+i/26)), "GBP", -2, 0)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, a.ID)
	}
	// A position on the day before, so the day's first movement builds
	// today's row from it and the rest move the row.
	if _, err := l.Project(cash.ID, day.AddDate(0, 0, -1), Fraction{Num: 0, Den: 1}); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		if _, err := l.RecordMovementWithProjections(cash.ID, id, Amount(100+i), CodeBookTransfer, day.Add(time.Duration(i)*time.Minute), "deposit"); err != nil {
			t.Fatal(err)
		}
	}
	want := Amount(0)
	for i := range ids {
		want -= Amount(100 + i)
	}
	p := mustPosition(t, l, cash.ID, day)
	if !p.Day.Equal(day) || p.Balance != want {
		t.Errorf("cash position %s %d, want %s %d", p.Day.Format("01-02"), p.Balance, day.Format("01-02"), want)
	}
	live, err := l.Balance(cash.ID)
	if err != nil || live != want {
		t.Errorf("ledger balance %d (%v), want %d", live, err, want)
	}
}

func BenchmarkHotAccountProjection(b *testing.B) {
	l, err := NewLedger(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	cash, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	to, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)
	day := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := l.RecordMovementWithProjections(cash.ID, to.ID, 100, CodeBookTransfer, day, "deposit"); err != nil {
			b.Fatal(err)
		}
	}
}
