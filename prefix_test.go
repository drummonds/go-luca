package luca

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// A general ledger beside the customer sub-ledger (gobank ADR-0005): two
// ledgers in one database, each with its own tables and contract views,
// neither seeing the other's accounts or movements.
func TestTwoLedgersShareOneDatabase(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sub, err := NewSQLLedger(db)
	if err != nil {
		t.Fatalf("sub-ledger: %v", err)
	}
	gl, err := NewSQLLedger(db, Prefix("gl_"))
	if err != nil {
		t.Fatalf("general ledger: %v", err)
	}
	assertLedgersIndependent(t, db, sub, gl, "gl_")
}

func assertLedgersIndependent(t *testing.T, db *sql.DB, sub, gl *SQLLedger, prefix string) {
	t.Helper()
	equity, err := sub.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}
	customer, err := sub.CreateAccount("Liability:Savings:C1", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}
	control, err := gl.CreateAccount("Liability:Savings:Control", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}
	glEquity, err := gl.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sub.RecordMovementWithProjections(equity.ID, customer.ID, 10000, CodeBookTransfer, at(day1, 9), "deposit"); err != nil {
		t.Fatal(err)
	}
	// The GL posts through a caller's transaction, as gobank does.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gl.WithTx(tx).RecordMovement(glEquity.ID, control.ID, 10000, CodeBookTransfer, at(day1, 9), "journal"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name  string
		l     *SQLLedger
		paths []string
		id    string
	}{
		{"sub-ledger", sub, []string{"Equity:Capital", "Liability:Savings:C1"}, customer.ID},
		{"general ledger", gl, []string{"Equity:Capital", "Liability:Savings:Control"}, control.ID},
	} {
		accounts, err := c.l.ListAccounts("")
		if err != nil {
			t.Fatal(err)
		}
		if len(accounts) != len(c.paths) {
			t.Errorf("%s: %d accounts, want %d", c.name, len(accounts), len(c.paths))
		}
		for i, a := range accounts {
			if i < len(c.paths) && a.FullPath != c.paths[i] {
				t.Errorf("%s: account %d = %s, want %s", c.name, i, a.FullPath, c.paths[i])
			}
		}
		bal, err := c.l.Balance(c.id)
		if err != nil {
			t.Fatal(err)
		}
		if bal != 10000 {
			t.Errorf("%s: balance %d, want 10000", c.name, bal)
		}
		if a, err := c.l.GetAccountByID(c.id); err != nil || a == nil {
			t.Errorf("%s: own account not found: %v", c.name, err)
		}
	}
	// Neither ledger resolves the other's account.
	if a, err := gl.GetAccountByID(customer.ID); err != nil || a != nil {
		t.Errorf("general ledger sees the sub-ledger's account: %v, %v", a, err)
	}
	if a, err := sub.GetAccountByID(control.ID); err != nil || a != nil {
		t.Errorf("sub-ledger sees the general ledger's account: %v, %v", a, err)
	}

	// Each ledger's contract views carry its name and only its rows.
	counts := map[string]int{
		"contract_ledger_movements":                    1,
		"contract_" + prefix + "ledger_movements":      1,
		"contract_ledger_eod_positions":                2,
		"contract_" + prefix + "ledger_eod_positions":  0,
		"contract_" + prefix + "ledger_live_positions": 2,
	}
	for view, want := range counts {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + view).Scan(&n); err != nil {
			t.Fatalf("%s: %v", view, err)
		}
		if n != want {
			t.Errorf("%s: %d rows, want %d", view, n, want)
		}
	}
	var glPath string
	if err := db.QueryRow(`SELECT to_path FROM contract_` + prefix + `ledger_movements`).Scan(&glPath); err != nil {
		t.Fatal(err)
	}
	if glPath != "Liability:Savings:Control" {
		t.Errorf("general ledger movement to %s", glPath)
	}
}

// The schema runs on every open, for both ledgers, over the views and
// indexes the last open left.
func TestPrefixedLedgerReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bank.db")
	open := func() (*sql.DB, *SQLLedger, *SQLLedger) {
		db, err := sql.Open("pglike", path)
		if err != nil {
			t.Fatal(err)
		}
		sub, err := NewSQLLedger(db)
		if err != nil {
			t.Fatalf("sub-ledger: %v", err)
		}
		gl, err := NewSQLLedger(db, Prefix("gl_"))
		if err != nil {
			t.Fatalf("general ledger: %v", err)
		}
		return db, sub, gl
	}
	db, sub, gl := open()
	assertLedgersIndependent(t, db, sub, gl, "gl_")
	_ = db.Close()
	db, _, gl = open()
	defer func() { _ = db.Close() }()
	accounts, err := gl.ListAccounts("")
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Errorf("general ledger after reopen: %d accounts, want 2", len(accounts))
	}
}

func TestPrefixMustBeAnIdentifier(t *testing.T) {
	for _, bad := range []string{"gl-", "1gl", "GL_", "gl ledger", "gl;"} {
		db, err := sql.Open("pglike", ":memory:")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := NewSQLLedger(db, Prefix(bad)); err == nil {
			t.Errorf("prefix %q accepted", bad)
		}
		_ = db.Close()
	}
}

// On PostgreSQL index names are unique per schema and a view cannot be
// dropped under another, so the prefixed set must create and reapply
// there too. Skipped without LUCA_TEST_PG_DSN.
func TestTwoLedgersShareOnePostgres(t *testing.T) {
	db := openEmptyPostgres(t)
	sub, err := NewSQLLedger(db)
	if err != nil {
		t.Fatalf("sub-ledger: %v", err)
	}
	gl, err := NewSQLLedger(db, Prefix("gl_"))
	if err != nil {
		t.Fatalf("general ledger: %v", err)
	}
	assertLedgersIndependent(t, db, sub, gl, "gl_")
	if _, err := NewSQLLedger(db, Prefix("gl_")); err != nil {
		t.Fatalf("reapply prefixed schema: %v", err)
	}
	if _, err := NewSQLLedger(db); err != nil {
		t.Fatalf("reapply schema: %v", err)
	}
	if _, err := gl.PositionAt(gl.mustAccount(t, "Liability:Savings:Control"), time.Now()); err != nil {
		t.Fatal(err)
	}
}

func (l *SQLLedger) mustAccount(t *testing.T, path string) string {
	t.Helper()
	a, err := l.GetAccount(path)
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}
