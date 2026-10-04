package luca

import (
	"database/sql"
	"testing"

	_ "git.bytestone.uk/hum3/go-postgres"
)

// CreateSchema populates any database/sql connection, so the same sample
// data is visible through the contract views whichever driver is behind it.
func TestCreateSchemaPopulatesContractViews(t *testing.T) {
	db, err := sql.Open("pglike", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := CreateSchema(db); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}
	var balance string
	err = db.QueryRow(`SELECT balance FROM contract_ledger_eod_positions WHERE full_path = $1`,
		"Asset:Bank:Savings:Main").Scan(&balance)
	if err != nil {
		t.Fatalf("eod position: %v", err)
	}
	if balance != "1000.12" {
		t.Fatalf("balance = %q, want 1000.12", balance)
	}
}

func TestCreateSchema(t *testing.T) {
	l, err := NewLedger(":memory:")
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	defer func() { _ = l.Close() }()
}

func TestCreateSchemaDB(t *testing.T) {
	db, err := CreateSchemaDB(":memory:")
	if err != nil {
		t.Fatalf("CreateSchemaDB: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Verify sample accounts were created
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM accounts").Scan(&count); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if count == 0 {
		t.Fatal("expected sample accounts, got 0")
	}

	// Verify sample movements were created
	if err := db.QueryRow("SELECT COUNT(*) FROM movements").Scan(&count); err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if count == 0 {
		t.Fatal("expected sample movements, got 0")
	}

	// Verify live balances
	if err := db.QueryRow("SELECT COUNT(*) FROM balances_live").Scan(&count); err != nil {
		t.Fatalf("count balances_live: %v", err)
	}
	if count == 0 {
		t.Fatal("expected sample live balances, got 0")
	}
}
