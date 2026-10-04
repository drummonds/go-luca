package luca

import (
	"testing"
	"time"
)

// A position is an account's end-of-day state: its balance and its
// accrued-but-unapplied interest as an exact fraction of minor units.
// Positions are stored projections; the ledger publishes them through the
// contract views contract_ledger_eod_positions and
// contract_ledger_live_positions.

var day1 = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

func at(day time.Time, hour int) time.Time { return day.Add(time.Duration(hour) * time.Hour) }

func mustPosition(t *testing.T, l *SQLLedger, accountID string, day time.Time) *Position {
	t.Helper()
	p, err := l.PositionAt(accountID, day)
	if err != nil {
		t.Fatalf("PositionAt(%s): %v", day.Format("2006-01-02"), err)
	}
	if p == nil {
		t.Fatalf("PositionAt(%s): no position", day.Format("2006-01-02"))
	}
	return p
}

func TestProjectionsCoverBothAccounts(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)

	if _, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 100000, CodeBookTransfer, at(day1, 10), "deposit"); err != nil {
		t.Fatal(err)
	}
	if got := mustPosition(t, l, savings.ID, day1).Balance; got != 100000 {
		t.Errorf("savings position = %d, want 100000", got)
	}
	if got := mustPosition(t, l, equity.ID, day1).Balance; got != -100000 {
		t.Errorf("equity position = %d, want -100000", got)
	}
}

func TestPositionAtTakesTheLatestRowOnOrBeforeTheDay(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)

	before, err := l.PositionAt(savings.ID, day1)
	if err != nil || before != nil {
		t.Fatalf("before any projection: %v, %v; want nil, nil", before, err)
	}
	if _, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 100000, CodeBookTransfer, at(day1, 10), "deposit"); err != nil {
		t.Fatal(err)
	}
	later := mustPosition(t, l, savings.ID, day1.AddDate(0, 0, 5))
	if !later.Day.Equal(day1) || later.Balance != 100000 {
		t.Errorf("five days on = %s %d, want %s 100000", later.Day.Format("2006-01-02"), later.Balance, day1.Format("2006-01-02"))
	}
}

func TestBackdatedMovementRewritesLaterProjections(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)
	day2, day3 := day1.AddDate(0, 0, 1), day1.AddDate(0, 0, 2)

	for _, m := range []struct {
		day    time.Time
		amount Amount
	}{{day1, 100}, {day3, 50}, {day2, 25}} { // day 2 arrives last, backdated
		if _, err := l.RecordMovementWithProjections(equity.ID, savings.ID, m.amount, CodeBookTransfer, at(m.day, 12), "deposit"); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []struct {
		day     time.Time
		balance Amount
	}{{day1, 100}, {day2, 125}, {day3, 175}} {
		if p := mustPosition(t, l, savings.ID, want.day); !p.Day.Equal(want.day) || p.Balance != want.balance {
			t.Errorf("savings %s = %s %d, want %d", want.day.Format("01-02"), p.Day.Format("01-02"), p.Balance, want.balance)
		}
		if p := mustPosition(t, l, equity.ID, want.day); p.Balance != -want.balance {
			t.Errorf("equity %s = %d, want %d", want.day.Format("01-02"), p.Balance, -want.balance)
		}
	}
}

func TestProjectWritesADayWithNoMovement(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)
	day2 := day1.AddDate(0, 0, 1)

	if _, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 100127, CodeBookTransfer, at(day1, 10), "deposit"); err != nil {
		t.Fatal(err)
	}
	accrued := Fraction{Num: 150000000, Den: 3650000}
	p, err := l.Project(savings.ID, day2, accrued)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if !p.Day.Equal(day2) || p.Balance != 100127 || p.Accrued != accrued {
		t.Errorf("projected = %+v", p)
	}
	if got := mustPosition(t, l, savings.ID, day2); got.Accrued != accrued || got.Balance != 100127 {
		t.Errorf("stored = %+v", got)
	}

	// A movement on day 1 after the projection rewrites day 2's balance and keeps its accrual.
	if _, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 1000, CodeBookTransfer, at(day1, 16), "late deposit"); err != nil {
		t.Fatal(err)
	}
	if got := mustPosition(t, l, savings.ID, day2); got.Balance != 101127 || got.Accrued != accrued {
		t.Errorf("after late deposit = %+v, want balance 101127 and accrual kept", got)
	}

	// Projecting the same day again replaces the accrual.
	again := Fraction{Num: 300000000, Den: 3650000}
	if _, err := l.Project(savings.ID, day2, again); err != nil {
		t.Fatal(err)
	}
	if got := mustPosition(t, l, savings.ID, day2); got.Accrued != again || got.Balance != 101127 {
		t.Errorf("reprojected = %+v", got)
	}
}

func TestKnowledgeTimeIsStoredOnEveryWritePath(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)

	m1, err := l.RecordMovement(equity.ID, savings.ID, 100, CodeBookTransfer, at(day1, 10), "plain")
	if err != nil {
		t.Fatal(err)
	}
	m2, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 100, CodeBookTransfer, at(day1, 11), "projected")
	if err != nil {
		t.Fatal(err)
	}
	batchID, err := l.RecordLinkedMovements([]MovementInput{{FromAccountID: equity.ID, ToAccountID: savings.ID, Amount: 100, Code: CodeBookTransfer}}, at(day1, 12))
	if err != nil {
		t.Fatal(err)
	}
	m3, err := l.AddMovementToBatch(batchID, MovementInput{FromAccountID: equity.ID, ToAccountID: savings.ID, Amount: 100, Code: CodeBookTransfer, Description: "batched"})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []*Movement{m1, m2, m3} {
		var stored string
		if err := l.db.QueryRow(`SELECT knowledge_time FROM movements WHERE id = $1`, m.ID).Scan(&stored); err != nil {
			t.Fatalf("read knowledge_time: %v", err)
		}
		got := parseDBTime(stored)
		if got.IsZero() {
			t.Fatalf("%s: knowledge_time %q did not parse", m.Description, stored)
		}
		// Microsecond precision is what PostgreSQL keeps; pglike keeps nanoseconds.
		if !got.Truncate(time.Microsecond).Equal(m.KnowledgeTime.Truncate(time.Microsecond)) {
			t.Errorf("%s: stored knowledge_time %s, returned %s", m.Description, got.Format(time.RFC3339Nano), m.KnowledgeTime.Format(time.RFC3339Nano))
		}
	}
}

func TestEODPositionsViewPublishesNumericMoney(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)
	day2 := day1.AddDate(0, 0, 1)

	if _, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 100127, CodeBookTransfer, at(day1, 10), "deposit"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Project(savings.ID, day2, Fraction{Num: 150000000, Den: 3650000}); err != nil {
		t.Fatal(err)
	}
	rows, err := l.db.Query(`SELECT full_path, balance, accrued FROM contract_ledger_eod_positions WHERE account_id = $1 ORDER BY day`, savings.ID)
	if err != nil {
		t.Fatalf("eod view: %v", err)
	}
	defer rows.Close()
	want := [][3]string{{"Liability:Savings:0001", "1001.27", "0.0000000"}, {"Liability:Savings:0001", "1001.27", "0.4109589"}}
	for i := 0; rows.Next(); i++ {
		var path, balance, accrued string
		if err := rows.Scan(&path, &balance, &accrued); err != nil {
			t.Fatal(err)
		}
		if i >= len(want) || [3]string{path, balance, accrued} != want[i] {
			t.Errorf("row %d = %q %q %q, want %v", i, path, balance, accrued, want[min(i, len(want)-1)])
		}
	}
}

func TestLivePositionsViewAddsMovementsSinceTheLatestProjection(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)
	day2 := day1.AddDate(0, 0, 1)

	if _, err := l.RecordMovementWithProjections(equity.ID, savings.ID, 100127, CodeBookTransfer, at(day1, 10), "deposit"); err != nil {
		t.Fatal(err)
	}
	// A plain movement the next day: no projection written.
	if _, err := l.RecordMovement(equity.ID, savings.ID, 1000, CodeBookTransfer, at(day2, 10), "deposit"); err != nil {
		t.Fatal(err)
	}
	var live, eod string
	if err := l.db.QueryRow(`SELECT balance FROM contract_ledger_live_positions WHERE account_id = $1`, savings.ID).Scan(&live); err != nil {
		t.Fatalf("live view: %v", err)
	}
	if live != "1011.27" {
		t.Errorf("live balance = %q, want 1011.27", live)
	}
	if err := l.db.QueryRow(`SELECT balance FROM contract_ledger_eod_positions WHERE account_id = $1 AND day = $2`, savings.ID, day1).Scan(&eod); err != nil {
		t.Fatalf("eod view: %v", err)
	}
	if eod != "1001.27" {
		t.Errorf("eod balance = %q, want 1001.27", eod)
	}
	// An account with no projection at all is live from its movements alone.
	if err := l.db.QueryRow(`SELECT balance FROM contract_ledger_live_positions WHERE account_id = $1`, equity.ID).Scan(&live); err != nil {
		t.Fatalf("live view (equity): %v", err)
	}
	if live != "-1011.27" {
		t.Errorf("equity live balance = %q, want -1011.27", live)
	}
}

func TestMovementsContractViewIsPublishedByTheLedger(t *testing.T) {
	l := newTestLedger(t)
	equity, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	savings, _ := l.CreateAccount("Liability:Savings:0001", "GBP", -2, 0)
	if _, err := l.RecordMovement(equity.ID, savings.ID, 100, CodeBookTransfer, at(day1, 10), "deposit"); err != nil {
		t.Fatal(err)
	}
	var fromPath, toPath string
	var amount Amount
	if err := l.db.QueryRow(`SELECT from_path, to_path, amount FROM contract_ledger_movements`).Scan(&fromPath, &toPath, &amount); err != nil {
		t.Fatalf("movements view: %v", err)
	}
	if fromPath != "Equity:Capital" || toPath != "Liability:Savings:0001" || amount != 100 {
		t.Errorf("got %s -> %s %d", fromPath, toPath, amount)
	}
}

func TestMemLedgerPositionsNotImplemented(t *testing.T) {
	m := NewMemLedger()
	if _, err := m.Project("x", day1, Fraction{}); err != ErrNotImplemented {
		t.Errorf("Project: %v", err)
	}
	if _, err := m.PositionAt("x", day1); err != ErrNotImplemented {
		t.Errorf("PositionAt: %v", err)
	}
}
