package luca

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkPositions measures what stage 3 of gobank ADR-0002 costs the
// ledger: a daily pass that writes one position per account per day, and
// the two contract views that publish them. The end-of-day view reads
// stored rows; the live view adds each account's movements since its latest
// position, so it is the dearer one. Runs on pglike :memory:; the gobank
// demo on Hetzner has ~300k accounts, so scale the per-account figures.
func BenchmarkPositions(b *testing.B) {
	for _, accounts := range []int{100, 1000} {
		const days = 30
		b.Run(fmt.Sprintf("accounts=%d/days=%d", accounts, days), func(b *testing.B) {
			l, err := NewLedger(":memory:")
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = l.Close() }()

			equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
			ids := make([]string, accounts)
			for i := range ids {
				a, err := l.CreateAccount(fmt.Sprintf("Liability:Savings:%06d", i), "GBP", -2, 0)
				if err != nil {
					b.Fatal(err)
				}
				ids[i] = a.ID
			}
			day0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			// Opening deposits, then a daily pass: one projection per account
			// per day, and a movement on one account in ten each day.
			for i, id := range ids {
				if _, err := l.RecordMovementWithProjections(equity.ID, id, 100000+Amount(i), CodeBookTransfer, day0.Add(9*time.Hour), "open"); err != nil {
					b.Fatal(err)
				}
			}
			b.Run("daily-pass", func(b *testing.B) {
				d := 1
				for i := 0; i < b.N; i++ {
					day := day0.AddDate(0, 0, d)
					for j, id := range ids {
						if j%10 == d%10 {
							if _, err := l.RecordMovementWithProjections(equity.ID, id, 250, CodeBookTransfer, day.Add(-12*time.Hour), "deposit"); err != nil {
								b.Fatal(err)
							}
						}
						if _, err := l.Project(id, day, Fraction{Num: int64(d) * 100000 * 150, Den: 3650000}); err != nil {
							b.Fatal(err)
						}
					}
					d++
				}
				b.ReportMetric(float64(len(ids)), "positions/op")
			})
			for d := 1; d <= days; d++ {
				day := day0.AddDate(0, 0, d)
				for _, id := range ids {
					if _, err := l.Project(id, day, Fraction{Num: int64(d) * 100000 * 150, Den: 3650000}); err != nil {
						b.Fatal(err)
					}
				}
			}
			var rows int
			if err := l.db.QueryRow(`SELECT COUNT(*) FROM balances_live`).Scan(&rows); err != nil {
				b.Fatal(err)
			}
			b.Logf("stored positions: %d (%d accounts × %d days)", rows, accounts, days+1)

			scan := func(b *testing.B, query string, args ...any) {
				b.Helper()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					rows, err := l.db.Query(query, args...)
					if err != nil {
						b.Fatal(err)
					}
					n := 0
					for rows.Next() {
						n++
					}
					rows.Close()
					if n == 0 {
						b.Fatal("no rows")
					}
				}
			}
			last := day0.AddDate(0, 0, days)
			b.Run("eod-view/all-accounts-one-day", func(b *testing.B) {
				scan(b, `SELECT account_id, balance, accrued FROM contract_ledger_eod_positions WHERE day = $1`, last)
			})
			b.Run("eod-view/one-account", func(b *testing.B) {
				scan(b, `SELECT balance, accrued FROM contract_ledger_eod_positions WHERE account_id = $1 AND day = $2`, ids[0], last)
			})
			b.Run("live-view/all-accounts", func(b *testing.B) {
				scan(b, `SELECT account_id, balance, accrued FROM contract_ledger_live_positions`)
			})
			b.Run("live-view/one-account", func(b *testing.B) {
				scan(b, `SELECT balance, accrued FROM contract_ledger_live_positions WHERE account_id = $1`, ids[0])
			})
			b.Run("position-at/one-account", func(b *testing.B) {
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := l.PositionAt(ids[0], last); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
