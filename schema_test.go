package luca

import (
	"testing"

	_ "codeberg.org/hum3/go-postgres"
)

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
