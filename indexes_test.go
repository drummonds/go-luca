package luca

import (
	"database/sql"
	"sort"
	"testing"
)

// indexNames lists the indexes in the database, on PostgreSQL or pglike.
func indexNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT indexname FROM pg_indexes WHERE schemaname = 'public'`)
	if err != nil {
		rows, err = db.Query(`SELECT name FROM sqlite_master WHERE type = 'index'`)
		if err != nil {
			t.Fatal(err)
		}
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func assertIndexes(t *testing.T, db *sql.DB, want ...string) {
	t.Helper()
	have := map[string]bool{}
	for _, n := range indexNames(t, db) {
		have[n] = true
	}
	for _, n := range want {
		if !have[n] {
			t.Errorf("index %s missing", n)
		}
	}
}

// A day's movements (the GL journal) and a day's positions (the GL
// reconciliation, the pass's unprojected list) are reads bounded by one
// day over tables of accounts × days, so each has an index leading with
// the day (issue #9). The schema adds them to an existing database on
// open, for a prefixed ledger too.
func TestDayBoundedReadsAreIndexed(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := NewSQLLedger(db); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLLedger(db, Prefix("gl_")); err != nil {
		t.Fatal(err)
	}
	assertIndexes(t, db,
		"idx_movements_value_time", "idx_balances_live_day",
		"idx_gl_movements_value_time", "idx_gl_balances_live_day")
}

func TestDayBoundedReadsAreIndexedOnPostgres(t *testing.T) {
	db := openEmptyPostgres(t)
	if _, err := NewSQLLedger(db); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSQLLedger(db, Prefix("gl_")); err != nil {
		t.Fatal(err)
	}
	assertIndexes(t, db,
		"idx_movements_value_time", "idx_balances_live_day",
		"idx_gl_movements_value_time", "idx_gl_balances_live_day")
}
