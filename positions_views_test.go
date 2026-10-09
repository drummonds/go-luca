package luca

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// The contract position views were first written with a correlated
// subquery per account (the latest projection, then the movements since
// it), which makes a whole-ledger read cost four index probes per account
// (issue #7). These are those definitions, kept as the oracle: a view
// written another way must publish the same rows. The latest view is
// grouped; the live view reads the business day's and the previous
// business day's slices of the day index (docs/queries.md).
const (
	referenceEODPositions = `
    SELECT p.account_id, a.full_path, a.commodity, p.balance_date AS day,
           round(p.balance::numeric / c.unit, -c.exponent) AS balance,
           round(p.accrued_num::numeric / p.accrued_den / c.unit, 7) AS accrued
    FROM balances_live p
    JOIN accounts a ON a.id = p.account_id
    JOIN commodities c ON c.code = a.commodity`

	referenceLatestPositions = referenceEODPositions + `
    WHERE p.balance_date = (SELECT MAX(q.balance_date) FROM balances_live q WHERE q.account_id = p.account_id)`

	referenceLivePositions = `
    SELECT l.account_id, l.full_path, l.commodity,
           round(l.balance_minor::numeric / l.unit, -l.exponent) AS balance,
           round(l.accrued_num::numeric / l.accrued_den / l.unit, 7) AS accrued
    FROM (
        SELECT a.id AS account_id, a.full_path, a.commodity, c.unit, c.exponent,
               COALESCE(p.balance, 0)
               + COALESCE((SELECT SUM(m.amount) FROM movements m
                           WHERE m.to_account_id = a.id
                             AND m.value_time >= COALESCE(p.next_day, '0001-01-01')), 0)
               - COALESCE((SELECT SUM(m.amount) FROM movements m
                           WHERE m.from_account_id = a.id
                             AND m.value_time >= COALESCE(p.next_day, '0001-01-01')), 0)
                 AS balance_minor,
               COALESCE(p.accrued_num, 0) AS accrued_num,
               COALESCE(p.accrued_den, 1) AS accrued_den
        FROM accounts a
        JOIN commodities c ON c.code = a.commodity
        LEFT JOIN balances_live p ON p.account_id = a.id
             AND p.balance_date = (SELECT MAX(q.balance_date) FROM balances_live q WHERE q.account_id = a.id)
    ) l`
)

// fillPartiallyProjectedLedger builds the state the views have to agree on
// mid-pass: some accounts projected to day D, some still on D-1, some
// never projected, movements before and after each account's latest
// projection, and two commodities so rounding is per account.
func fillPartiallyProjectedLedger(t testing.TB, l *SQLLedger, savers int) {
	t.Helper()
	equity, err := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}
	yen, err := l.CreateAccount("Equity:Capital:JPY", "JPY", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	dayD := day1.AddDate(0, 0, 2)
	for i := range savers {
		from, commodity, exponent := equity, "GBP", -2
		if i%7 == 3 {
			from, commodity, exponent = yen, "JPY", 0
		}
		a, err := l.CreateAccount(fmt.Sprintf("Liability:Savings:%06d", i), commodity, exponent, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := l.RecordMovementWithProjections(from.ID, a.ID, 100000+Amount(i)*7, CodeBookTransfer, at(day1, 9), "open"); err != nil {
			t.Fatal(err)
		}
		accrued := Fraction{Num: int64(1000 + i), Den: 365}
		switch i % 4 {
		case 0: // projected to D, then a movement the day after
			if _, err := l.Project(a.ID, dayD, accrued); err != nil {
				t.Fatal(err)
			}
			if _, err := l.RecordMovement(from.ID, a.ID, 250+Amount(i), CodeBookTransfer, at(dayD.AddDate(0, 0, 1), 10), "deposit"); err != nil {
				t.Fatal(err)
			}
		case 1: // still on D-1 (mid-pass), with a movement on D and a withdrawal
			if _, err := l.Project(a.ID, dayD.AddDate(0, 0, -1), accrued); err != nil {
				t.Fatal(err)
			}
			if _, err := l.RecordMovement(from.ID, a.ID, 500, CodeBookTransfer, at(dayD, 11), "deposit"); err != nil {
				t.Fatal(err)
			}
			if _, err := l.RecordMovement(a.ID, from.ID, 125, CodeBookTransfer, at(dayD, 12), "withdrawal"); err != nil {
				t.Fatal(err)
			}
		case 2: // projected to D, nothing since
			if _, err := l.Project(a.ID, dayD, accrued); err != nil {
				t.Fatal(err)
			}
		case 3: // never projected beyond the opening movement's own day
		}
	}
	// The pass is on day D.
	if err := l.AdvanceDay(dayD); err != nil {
		t.Fatal(err)
	}
	// An account with no projection at all, live from its movements alone.
	loose, err := l.CreateAccount("Asset:Suspense", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.RecordMovement(equity.ID, loose.ID, 999, CodeBookTransfer, at(dayD, 8), "suspense"); err != nil {
		t.Fatal(err)
	}
}

func readRows(t testing.TB, db dbtx, query string) [][]string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("%v\n%s", err, query)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			row[i] = v.String
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertViewMatchesReference(t *testing.T, db dbtx, view, reference, order string) {
	t.Helper()
	got := readRows(t, db, fmt.Sprintf("SELECT * FROM %s ORDER BY %s", view, order))
	want := readRows(t, db, fmt.Sprintf("SELECT * FROM (%s) r ORDER BY %s", reference, order))
	if len(got) == 0 {
		t.Fatalf("%s: no rows", view)
	}
	if len(got) != len(want) {
		t.Fatalf("%s: %d rows, reference %d", view, len(got), len(want))
	}
	for i := range got {
		if fmt.Sprint(got[i]) != fmt.Sprint(want[i]) {
			t.Errorf("%s row %d = %v, reference %v", view, i, got[i], want[i])
		}
	}
}

func assertPositionViewsMatchReference(t *testing.T, db dbtx) {
	t.Helper()
	assertViewMatchesReference(t, db, "contract_ledger_eod_positions", referenceEODPositions, "account_id, day")
	assertViewMatchesReference(t, db, "contract_ledger_latest_positions", referenceLatestPositions, "account_id")
	assertViewMatchesReference(t, db, "contract_ledger_live_positions", referenceLivePositions, "account_id")
}

func TestPositionViewsMatchReferenceDefinitions(t *testing.T) {
	l := newTestLedger(t)
	fillPartiallyProjectedLedger(t, l, 23)
	assertPositionViewsMatchReference(t, l.db)
}

// The one-account read gobank's statement and product engine make must
// filter the same rows the whole-ledger read publishes.
func TestPositionViewsFilterByAccount(t *testing.T) {
	l := newTestLedger(t)
	fillPartiallyProjectedLedger(t, l, 9)
	var id string
	if err := l.db.QueryRow(`SELECT id FROM accounts WHERE full_path = 'Liability:Savings:000001'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"contract_ledger_latest_positions", "contract_ledger_live_positions"} {
		got := readRows(t, l.db, fmt.Sprintf("SELECT balance, accrued FROM %s WHERE account_id = '%s'", view, id))
		if len(got) != 1 {
			t.Fatalf("%s: %d rows for one account", view, len(got))
		}
	}
	live := readRows(t, l.db, fmt.Sprintf("SELECT balance FROM contract_ledger_live_positions WHERE account_id = '%s'", id))
	// opened 1000.07, projected on D-1, +5.00 and -1.25 on D
	if live[0][0] != "1003.82" {
		t.Errorf("live balance = %q, want 1003.82", live[0][0])
	}
}

// Every consumer that wanted "the latest day per account" bolted its own
// correlated MAX(day) onto the end-of-day view. The latest view is that
// row, once, and it is the position the live view starts from.
func TestLatestPositionsViewIsOneRowPerProjectedAccount(t *testing.T) {
	l := newTestLedger(t)
	fillPartiallyProjectedLedger(t, l, 8)
	rows := readRows(t, l.db, `SELECT account_id, day FROM contract_ledger_latest_positions ORDER BY account_id`)
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r[0]] {
			t.Errorf("account %s twice", r[0])
		}
		seen[r[0]] = true
		var latest string
		if err := l.db.QueryRow(`SELECT MAX(balance_date) FROM balances_live WHERE account_id = $1`, r[0]).Scan(&latest); err != nil {
			t.Fatal(err)
		}
		if parseDBTime(r[1]) != parseDBTime(latest) {
			t.Errorf("account %s latest day %s, want %s", r[0], r[1], latest)
		}
	}
	var projected int
	if err := l.db.QueryRow(`SELECT COUNT(DISTINCT account_id) FROM balances_live`).Scan(&projected); err != nil {
		t.Fatal(err)
	}
	if len(rows) != projected {
		t.Errorf("%d rows, %d projected accounts", len(rows), projected)
	}
}

// The schema runs on every open, so the views must drop and recreate in an
// order PostgreSQL accepts when one view reads another.
func TestSchemaReappliesOverExistingViews(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.db")
	first, err := NewLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	fillPartiallyProjectedLedger(t, first, 5)
	_ = first.Close()
	second, err := NewLedger(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = second.Close() }()
	assertPositionViewsMatchReference(t, second.db)
}

// The same checks on PostgreSQL, when LUCA_TEST_PG_DSN points at one
// (task bench:pg:start gives a local container). Skipped otherwise.
func TestPositionViewsMatchReferenceOnPostgres(t *testing.T) {
	db := openEmptyPostgres(t)
	l, err := NewSQLLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	fillPartiallyProjectedLedger(t, l, 23)
	assertPositionViewsMatchReference(t, db)
	// Reapply over the views just created.
	if _, err := NewSQLLedger(db); err != nil {
		t.Fatalf("reapply schema: %v", err)
	}
	assertPositionViewsMatchReference(t, db)
}

// openEmptyPostgres connects to LUCA_TEST_PG_DSN and empties its public
// schema, or skips the test when the variable is unset.
func openEmptyPostgres(t testing.TB) *sql.DB {
	t.Helper()
	dsn := os.Getenv("LUCA_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("LUCA_TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	return db
}

// openBenchLedger is a fresh ledger on PostgreSQL when LUCA_TEST_PG_DSN is
// set, else on pglike :memory:.
func openBenchLedger(b *testing.B) *SQLLedger {
	b.Helper()
	if os.Getenv("LUCA_TEST_PG_DSN") != "" {
		l, err := NewSQLLedger(openEmptyPostgres(b))
		if err != nil {
			b.Fatal(err)
		}
		return l
	}
	l, err := NewLedger(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = l.Close() })
	return l
}

// backfillPositionHistory writes days of earlier positions for every
// account, straight into balances_live, so the fixture has the depth of a
// long-running ledger (the gobank demo is past day 400). The correlated
// reference re-ran its MAX subquery once per stored row per account, so
// this depth is what made it fail at scale.
func backfillPositionHistory(b *testing.B, l *SQLLedger, days int) {
	b.Helper()
	ids := readRows(b, l.db, `SELECT id FROM accounts`)
	tx, commit, _, err := l.begin()
	if err != nil {
		b.Fatal(err)
	}
	const batch = 400
	var sb strings.Builder
	args := make([]any, 0, batch*7)
	flush := func() {
		if len(args) == 0 {
			return
		}
		if _, err := tx.Exec(`INSERT INTO balances_live (id, account_id, balance_date, next_day, balance, accrued_num, accrued_den) VALUES `+sb.String(), args...); err != nil {
			b.Fatal(err)
		}
		sb.Reset()
		args = args[:0]
	}
	for _, id := range ids {
		for d := 1; d <= days; d++ {
			if len(args) > 0 {
				sb.WriteString(",")
			}
			n := len(args)
			fmt.Fprintf(&sb, "($%d,$%d,$%d,$%d,$%d,$%d,$%d)", n+1, n+2, n+3, n+4, n+5, n+6, n+7)
			day := day1.AddDate(0, 0, -d)
			args = append(args, uuid.New().String(), id[0], utc(day), utc(day.AddDate(0, 0, 1)), Amount(100000-d), int64(d), int64(365))
			if len(args) >= batch*7 {
				flush()
			}
		}
	}
	flush()
	if err := commit(); err != nil {
		b.Fatal(err)
	}
}

// BenchmarkPositionViewsScale measures a whole-ledger read of the position
// views against the correlated-subquery reference, at two scales an order
// of magnitude apart, whole ledger and one account: the figures in
// docs/queries.md. On pglike :memory:, or on PostgreSQL when
// LUCA_TEST_PG_DSN is set, which is where the correlated form was measured
// failing (issue #7).
func BenchmarkPositionViewsScale(b *testing.B) {
	for _, accounts := range []int{1000, 10000} {
		b.Run(fmt.Sprintf("accounts=%d", accounts), func(b *testing.B) {
			l := openBenchLedger(b)
			fillPartiallyProjectedLedger(b, l, accounts)
			backfillPositionHistory(b, l, 90)
			// What autovacuum would have told the planner about the load:
			// without it PostgreSQL priced the day index at twenty times
			// its cost and scanned the positions table instead.
			if _, err := l.db.Exec(`ANALYZE`); err != nil {
				b.Fatal(err)
			}
			var one string
			if err := l.db.QueryRow(`SELECT id FROM accounts WHERE full_path = 'Liability:Savings:000001'`).Scan(&one); err != nil {
				b.Fatal(err)
			}
			scan := func(b *testing.B, query string, perAccount bool) {
				b.Helper()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					rows := readRows(b, l.db, query)
					if len(rows) == 0 {
						b.Fatal("no rows")
					}
				}
				if perAccount {
					b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(accounts)/1e3, "us/account")
				}
			}
			views := []struct{ name, sql string }{
				{"live-view", "contract_ledger_live_positions"},
				{"live-reference", "(" + referenceLivePositions + ") r"},
				{"latest-view", "contract_ledger_latest_positions"},
				{"latest-reference", "(" + referenceLatestPositions + ") r"},
			}
			for _, v := range views {
				b.Run(v.name+"/all-accounts", func(b *testing.B) {
					scan(b, `SELECT account_id, balance, accrued FROM `+v.sql, true)
				})
				b.Run(v.name+"/one-account", func(b *testing.B) {
					scan(b, `SELECT account_id, balance, accrued FROM `+v.sql+` WHERE account_id = '`+one+`'`, false)
				})
			}
		})
	}
}
