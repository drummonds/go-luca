package luca

import (
	"fmt"
	"testing"
	"time"

	_ "codeberg.org/hum3/go-postgres"
)

func BenchmarkRecordMovement(b *testing.B) {
	l, err := NewLedger(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	from, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	to, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := l.RecordMovement(from.ID, to.ID, 10000, CodeBookTransfer, now, "benchmark")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBalanceQuery(b *testing.B) {
	for _, movementCount := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("movements=%d", movementCount), func(b *testing.B) {
			l, err := NewLedger(":memory:")
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = l.Close() }()

			from, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
			to, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)

			for i := range movementCount {
				t := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Hour)
				if _, err := l.RecordMovement(from.ID, to.ID, 100, CodeBookTransfer, t, ""); err != nil {
					b.Fatalf("RecordMovement: %v", err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := l.Balance(to.ID)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSimpleMovementAndBalance(b *testing.B) {
	l, err := NewLedger(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.05)

	// Initial deposit
	if _, err := l.RecordMovement(equity.ID, savings.ID, 100000, CodeBookTransfer,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "initial"); err != nil {
		b.Fatalf("RecordMovement: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute)
		_, err := l.RecordMovement(equity.ID, savings.ID, 1000, CodeBookTransfer, t, "deposit")
		if err != nil {
			b.Fatal(err)
		}
		_, err = l.Balance(savings.ID)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompoundMovementWithProjections(b *testing.B) {
	l, err := NewLedger(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = l.Close() }()

	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0.05)

	// Initial deposit
	if _, err := l.RecordMovement(equity.ID, savings.ID, 100000, CodeBookTransfer,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "initial"); err != nil {
		b.Fatalf("RecordMovement: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute)
		_, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 1000, CodeBookTransfer, t, "deposit")
		if err != nil {
			b.Fatal(err)
		}
	}
}
