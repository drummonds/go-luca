package luca

import (
	"testing"

	_ "github.com/drummonds/go-postgres"
)

func TestCreateSchema(t *testing.T) {
	l, err := NewLedger(":memory:")
	if err != nil {
		t.Fatalf("NewLedger: %v", err)
	}
	defer l.Close()
}
