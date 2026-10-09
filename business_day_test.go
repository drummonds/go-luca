package luca

import (
	"database/sql"
	"testing"
)

// The business day is the day the pass is projecting. The live view reads
// each account's position on it, else on the previous business day (the
// pass has not reached the account yet), so the two days together are
// where every account's latest projection is while the ledger is kept up.
func TestAdvanceDayKeepsThePreviousBusinessDay(t *testing.T) {
	testAdvanceDay(t, newTestLedger(t))
}

func TestAdvanceDayOnPostgres(t *testing.T) {
	l, err := NewSQLLedger(openEmptyPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	testAdvanceDay(t, l)
}

func testAdvanceDay(t *testing.T, l *SQLLedger) {
	t.Helper()
	before, err := l.BusinessDay()
	if err != nil {
		t.Fatal(err)
	}
	if before != nil {
		t.Fatalf("business day before any advance = %v, want none", before)
	}

	dayD := day1.AddDate(0, 0, 2)
	if err := l.AdvanceDay(at(dayD, 9)); err != nil {
		t.Fatal(err)
	}
	got, err := l.BusinessDay()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.Day.Equal(dayD) || !got.PrevDay.IsZero() {
		t.Fatalf("after the first advance = %+v, want day %s at midnight and no previous day", got, dayD)
	}

	// The same day again: the pass restarting. Nothing moves.
	if err := l.AdvanceDay(dayD); err != nil {
		t.Fatal(err)
	}
	got, _ = l.BusinessDay()
	if !got.Day.Equal(dayD) || !got.PrevDay.IsZero() {
		t.Errorf("after advancing to the same day = %+v, want unchanged", got)
	}

	// A skipped day: the previous business day is the last one advanced
	// to, not the calendar day before, because that is where the
	// unprojected accounts' positions are.
	dayD2 := dayD.AddDate(0, 0, 2)
	if err := l.AdvanceDay(dayD2); err != nil {
		t.Fatal(err)
	}
	got, _ = l.BusinessDay()
	if !got.Day.Equal(dayD2) || !got.PrevDay.Equal(dayD) {
		t.Errorf("after skipping a day = %+v, want day %s previous %s", got, dayD2, dayD)
	}

	// The ledger does not go back in time.
	if err := l.AdvanceDay(dayD); err == nil {
		t.Error("advancing to an earlier day: want an error")
	}
	got, _ = l.BusinessDay()
	if !got.Day.Equal(dayD2) || !got.PrevDay.Equal(dayD) {
		t.Errorf("after a refused advance = %+v, want unchanged", got)
	}
}

// Before the pass has ever advanced the day, and after it has moved on
// past every projection, the live view still publishes the same rows as
// the correlated reference: those accounts take the latest-row path.
func TestLivePositionsViewWithoutABusinessDayRow(t *testing.T) {
	l := newTestLedger(t)
	fillPartiallyProjectedLedger(t, l, 13)
	if _, err := l.db.Exec(`DELETE FROM ledger_day`); err != nil {
		t.Fatal(err)
	}
	assertPositionViewsMatchReference(t, l.db)
}

func TestLivePositionsViewAfterThePassMovesOn(t *testing.T) {
	l := newTestLedger(t)
	fillPartiallyProjectedLedger(t, l, 13)
	bd, err := l.BusinessDay()
	if err != nil {
		t.Fatal(err)
	}
	// Day D+1 opens with nothing projected yet: every account is on the
	// previous business day or earlier.
	if err := l.AdvanceDay(bd.Day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	assertPositionViewsMatchReference(t, l.db)
	// And once the pass skips a day, D-1's accounts are off both days.
	if err := l.AdvanceDay(bd.Day.AddDate(0, 0, 3)); err != nil {
		t.Fatal(err)
	}
	assertPositionViewsMatchReference(t, l.db)
}

// A second ledger in the database keeps its own business day.
func TestBusinessDayIsPerPrefixedLedger(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sub, err := NewSQLLedger(db)
	if err != nil {
		t.Fatal(err)
	}
	gl, err := NewSQLLedger(db, Prefix("gl_"))
	if err != nil {
		t.Fatal(err)
	}
	if err := sub.AdvanceDay(day1); err != nil {
		t.Fatal(err)
	}
	got, err := gl.BusinessDay()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("prefixed ledger's business day = %+v, want none", got)
	}
}
