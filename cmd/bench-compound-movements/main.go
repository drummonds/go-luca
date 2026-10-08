package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"git.bytestone.uk/hum3/go-luca/internal/benchutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

const schemaDDL = `
CREATE TABLE IF NOT EXISTS accounts (
    id SERIAL PRIMARY KEY,
    full_path VARCHAR(500) NOT NULL UNIQUE,
    account_type VARCHAR(50) NOT NULL,
    product VARCHAR(100) NOT NULL DEFAULT '',
    account_id VARCHAR(100) NOT NULL DEFAULT '',
    address VARCHAR(100) NOT NULL DEFAULT '',
    is_pending BOOLEAN DEFAULT FALSE,
    currency VARCHAR(10) NOT NULL DEFAULT 'GBP',
    exponent INTEGER NOT NULL DEFAULT -2,
    annual_interest_rate NUMERIC(10,6) NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS movements (
    id SERIAL PRIMARY KEY,
    batch_id INTEGER NOT NULL,
    from_account_id INTEGER NOT NULL,
    to_account_id INTEGER NOT NULL,
    amount BIGINT NOT NULL,
    code VARCHAR(14) NOT NULL,
    ledger INTEGER NOT NULL DEFAULT 0,
    pending_id BIGINT NOT NULL DEFAULT 0,
    user_data_64 BIGINT NOT NULL DEFAULT 0,
    value_time TIMESTAMP NOT NULL,
    knowledge_time TIMESTAMP DEFAULT NOW(),
    description VARCHAR(500) NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_movements_from ON movements(from_account_id, value_time);
CREATE INDEX IF NOT EXISTS idx_movements_to ON movements(to_account_id, value_time);
CREATE INDEX IF NOT EXISTS idx_movements_batch ON movements(batch_id);
CREATE INDEX IF NOT EXISTS idx_movements_code ON movements(to_account_id, code, value_time);

CREATE TABLE IF NOT EXISTS balances_live (
    id SERIAL PRIMARY KEY,
    account_id INTEGER NOT NULL,
    balance_date TIMESTAMP NOT NULL,
    next_day TIMESTAMP NOT NULL,
    balance BIGINT NOT NULL,
    accrued_num BIGINT NOT NULL DEFAULT 0,
    accrued_den BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_balances_live_unique
    ON balances_live(account_id, balance_date);
CREATE INDEX IF NOT EXISTS idx_balances_live_day
    ON balances_live(balance_date, account_id);

-- The ledger's business day: one row, advanced at the start of each day.
CREATE TABLE IF NOT EXISTS ledger_day (
    day TIMESTAMP NOT NULL,
    prev_day TIMESTAMP NOT NULL
);

-- Live positions by business day: today's row, else yesterday's (the pass
-- has not reached the account), else the latest (stale), plus the movements
-- valued on or after that row's next_day.
CREATE VIEW live_positions AS
    SELECT l.account_id,
           l.balance
           + COALESCE((SELECT SUM(m.amount) FROM movements m
                       WHERE m.to_account_id = l.account_id AND m.value_time >= l.next_day), 0)
           - COALESCE((SELECT SUM(m.amount) FROM movements m
                       WHERE m.from_account_id = l.account_id AND m.value_time >= l.next_day), 0) AS balance
    FROM (
        SELECT a.id AS account_id,
               COALESCE(p.balance, y.balance,
                        (SELECT z.balance FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1),
                        0) AS balance,
               COALESCE(p.next_day, y.next_day,
                        (SELECT z.next_day FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1),
                        '0001-01-01') AS next_day
        FROM accounts a
        CROSS JOIN ledger_day d
        LEFT JOIN balances_live p ON p.account_id = a.id AND p.balance_date = d.day
        LEFT JOIN balances_live y ON y.account_id = a.id AND y.balance_date = d.prev_day
    ) l;

-- The same, with the business day read through a function instead of a
-- join to the row.
CREATE FUNCTION business_day() RETURNS TIMESTAMP LANGUAGE sql STABLE
    AS 'SELECT day FROM ledger_day';
CREATE FUNCTION business_prev_day() RETURNS TIMESTAMP LANGUAGE sql STABLE
    AS 'SELECT prev_day FROM ledger_day';
CREATE VIEW live_positions_fn AS
    SELECT l.account_id,
           l.balance
           + COALESCE((SELECT SUM(m.amount) FROM movements m
                       WHERE m.to_account_id = l.account_id AND m.value_time >= l.next_day), 0)
           - COALESCE((SELECT SUM(m.amount) FROM movements m
                       WHERE m.from_account_id = l.account_id AND m.value_time >= l.next_day), 0) AS balance
    FROM (
        SELECT a.id AS account_id,
               COALESCE(p.balance, y.balance,
                        (SELECT z.balance FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1),
                        0) AS balance,
               COALESCE(p.next_day, y.next_day,
                        (SELECT z.next_day FROM balances_live z WHERE z.account_id = a.id ORDER BY z.balance_date DESC LIMIT 1),
                        '0001-01-01') AS next_day
        FROM accounts a
        LEFT JOIN balances_live p ON p.account_id = a.id AND p.balance_date = business_day()
        LEFT JOIN balances_live y ON y.account_id = a.id AND y.balance_date = business_prev_day()
    ) l;

-- Live positions by latest row, found with a correlated MAX per account:
-- go-luca v0.3's contract_ledger_live_positions, what the compound
-- approach reads.
CREATE VIEW live_positions_latest AS
    SELECT l.account_id,
           l.balance
           + COALESCE((SELECT SUM(m.amount) FROM movements m
                       WHERE m.to_account_id = l.account_id AND m.value_time >= l.next_day), 0)
           - COALESCE((SELECT SUM(m.amount) FROM movements m
                       WHERE m.from_account_id = l.account_id AND m.value_time >= l.next_day), 0) AS balance
    FROM (
        SELECT a.id AS account_id, COALESCE(p.balance, 0) AS balance,
               COALESCE(p.next_day, '0001-01-01') AS next_day
        FROM accounts a
        LEFT JOIN balances_live p ON p.account_id = a.id
             AND p.balance_date = (SELECT MAX(q.balance_date) FROM balances_live q WHERE q.account_id = a.id)
    ) l;
`

// Whole-ledger live reads, one per approach. Simple sums every movement;
// compound takes each account's latest position row; position takes the
// business day's row.
var liveReads = []struct{ label, sql string }{
	{"simple (sum of movements)", `
    SELECT a.id, COALESCE(ti.amount, 0) - COALESCE(fo.amount, 0) AS balance
    FROM accounts a
    LEFT JOIN (SELECT to_account_id AS id, SUM(amount) AS amount FROM movements GROUP BY to_account_id) ti ON ti.id = a.id
    LEFT JOIN (SELECT from_account_id AS id, SUM(amount) AS amount FROM movements GROUP BY from_account_id) fo ON fo.id = a.id`},
	{"compound (latest row by MAX)", `SELECT account_id, balance FROM live_positions_latest`},
	{"position (business-day row)", `SELECT account_id, balance FROM live_positions`},
	{"position (business-day function)", `SELECT account_id, balance FROM live_positions_fn`},
}

type scenario struct {
	n int // seed movements per savings account
	m int // number of savings accounts
}

var scenarios = []scenario{
	{100, 10},       // 1K movements
	{1_000, 100},    // 100K movements
	{10_000, 100},   // 1M movements, 417 days of positions
	{1_000, 1_000},  // 1M movements, 1K accounts
	{10_000, 1_000}, // 10M movements, 1K accounts, 417 days of positions
}

// readIterations is how many times each whole-ledger read runs: they are
// seconds each on the largest scenario.
const readIterations = 10

const seedAmount = 1000

const (
	equityID          = 1
	expenseInterestID = 2
	firstSavingsID    = 3
	annualRate        = 0.05
	exponent          = -2
)

func main() {
	ctx := context.Background()

	fmt.Println("Starting PostgreSQL...")
	pg, err := benchutil.StartPG(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pg.Stop(ctx)
	fmt.Println("PostgreSQL ready.")

	report := benchutil.NewReport("Compound Movements",
		"Compare simple, compound and position movement throughput, and the whole-ledger live read each leaves behind, on real PostgreSQL")
	report.AddDBInfo(pg.DSN, pg.IsContainer())

	report.AddSQL("Simple operation (single tx)", `BEGIN;
SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements;
INSERT INTO movements (batch_id, from_account_id, to_account_id, amount,
    code, value_time, description)
  VALUES ($1, $2, $3, $4, 'PMNT:RCDT:BOOK', $5, 'deposit')
  RETURNING id;
SELECT
  COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1), 0)
- COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $1), 0);
COMMIT;`)

	report.AddSQL("Compound operation (single tx)", `BEGIN;
SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements;
INSERT INTO movements (...) VALUES (...) RETURNING id;
SELECT ... SUM ... WHERE value_time <= eod;           -- eod balance
-- (Go: interest = balance * rate / 365)
DELETE FROM movements WHERE to_account_id=$1 AND code='LDAS:FTDP:INTR'
  AND value_time >= $2 AND value_time <= $3;           -- old accrual
INSERT INTO movements (...) VALUES (...);              -- new accrual
DELETE FROM balances_live WHERE account_id=$1
  AND balance_date=$2;                                 -- old live
SELECT ... SUM ... WHERE value_time <= eod;            -- recompute
INSERT INTO balances_live (...) VALUES (...);           -- new live
COMMIT;`)

	report.AddSQL("Position operation (single tx)", `BEGIN;
SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements;
INSERT INTO movements (...) VALUES (...) RETURNING id;
SELECT balance FROM balances_live WHERE account_id=$1
  AND balance_date < $day ORDER BY balance_date DESC LIMIT 1;   -- previous position
SELECT ... SUM ... WHERE value_time >= $day AND value_time < $next; -- the day's movements
INSERT INTO balances_live (account_id, balance_date, next_day, balance)
  VALUES (...) ON CONFLICT (account_id, balance_date)
  DO UPDATE SET balance = EXCLUDED.balance;                     -- today's position
SELECT balance FROM live_positions WHERE account_id = $1;      -- live read
COMMIT;`)

	report.AddSQL("Live positions by business day (view)", `SELECT a.id AS account_id,
       COALESCE(p.balance, y.balance, (latest row), 0)
       + movements valued >= that row's next_day
FROM accounts a
CROSS JOIN ledger_day d                                   -- one row: day, prev_day
LEFT JOIN balances_live p ON p.account_id = a.id AND p.balance_date = d.day
LEFT JOIN balances_live y ON y.account_id = a.id AND y.balance_date = d.prev_day;
-- index: balances_live(balance_date, account_id)
-- the function variant replaces d.day / d.prev_day with business_day() / business_prev_day()`)

	report.AddMethods(
		"- **Approaches:** Simple (insert + balance) vs Compound (insert + interest projection + live balance) vs Position (insert + today's position row + live read by business day)\n" +
			"- **Schema:** Same as go-luca schema.go (accounts, movements, balances_live)\n" +
			"- **N:** Seed movements per savings account\n" +
			"- **M:** Number of savings accounts (plus 1 equity + 1 expense:interest)\n" +
			"- **Seed data:** N movements per savings account from equity, one an hour, loaded via pgx CopyFrom in 10K-row batches; one position row per account per seeded day (N/24 days deep) for every approach, so each reads over the same history\n" +
			"- **Business day:** the day after the last seeded movement, held in the one-row `ledger_day` table; every approach writes on that day\n" +
			"- **Interest:** 5% annual rate, exponent -2, computed via shopspring/decimal\n" +
			"- **Iteration target:** Round-robin savings accounts, unique value_time (same day, different minutes)\n" +
			"- **Timing:** Per-iteration wall-clock via benchutil.RunTimed\n" +
			"- **Warmup:** None — first iteration included\n" +
			"- **Transaction:** Each approach's write wrapped in an explicit pgx transaction\n" +
			fmt.Sprintf("- **Whole-ledger live read:** every account's live balance in one query, %d iterations per approach, after that approach's writes", readIterations))

	benchDir := "benchmarks/compound-movements"

	for _, sc := range scenarios {
		totalMov := sc.n * sc.m
		fmt.Printf("\n=== N=%s mvts/acct, M=%s accounts (%s total movements) ===\n",
			benchutil.FmtInt(sc.n), benchutil.FmtInt(sc.m), benchutil.FmtInt(totalMov))

		var writes, reads []*benchutil.TimingResult
		day := businessDay(sc.n)
		approaches := []struct {
			name string
			run  func(ctx context.Context, pool *pgxpool.Pool, acctID int, vt time.Time) error
			read int // index into liveReads
		}{
			{"simple", runSimple, 0},
			{"compound", runCompound, 1},
			{"position", runPosition, 2},
		}
		for _, ap := range approaches {
			fmt.Printf("  Seeding for %s benchmark...\n", ap.name)
			if err := resetAndSeed(ctx, pg.Pool, sc.n, sc.m); err != nil {
				log.Fatalf("seed %s: %v", ap.name, err)
			}
			iter := 0
			r, err := benchutil.RunTimed(ap.name, sc.n, sc.m, 0, func() error {
				acctID := firstSavingsID + (iter % sc.m)
				vt := day.Add(time.Duration(iter) * time.Minute)
				iter++
				return ap.run(ctx, pg.Pool, acctID, vt)
			})
			if err != nil {
				log.Fatalf("bench %s: %v", ap.name, err)
			}
			fmt.Printf("  %-9s mean=%-10s p50=%-10s p99=%s  TPS=%s\n", ap.name+":",
				fmtDur(r.Mean), fmtDur(r.P50), fmtDur(r.P99), benchutil.FmtInt(int(time.Second/r.Mean)))
			writes = append(writes, r)

			readsFor := []int{ap.read}
			if ap.name == "position" {
				readsFor = append(readsFor, 3)
			}
			for _, ri := range readsFor {
				lr := liveReads[ri]
				r, err := benchutil.RunTimed(lr.label, sc.n, sc.m, readIterations, func() error {
					return scanAll(ctx, pg.Pool, lr.sql)
				})
				if err != nil {
					log.Fatalf("read %s: %v", lr.label, err)
				}
				fmt.Printf("    live read %-34s mean=%-10s p99=%s\n", lr.label, fmtDur(r.Mean), fmtDur(r.P99))
				reads = append(reads, r)
			}
		}

		label := fmt.Sprintf("N=%s, M=%s", benchutil.FmtInt(sc.n), benchutil.FmtInt(sc.m))
		report.AddTPSResults(label, writes)
		report.AddResults("whole-ledger live read, "+label, reads)
	}

	report.AddFileSection("Purpose", filepath.Join(benchDir, "purpose.md"))
	report.AddFileSection("Analysis", filepath.Join(benchDir, "analysis.md"))
	report.AddFileSection("AI Summary", filepath.Join(benchDir, "summary.md"))

	path, err := report.Write("compound-movements")
	if err != nil {
		log.Fatalf("write report: %v", err)
	}
	fmt.Printf("\nReport written to: %s\n", path)
}

func resetAndSeed(ctx context.Context, pool *pgxpool.Pool, n, m int) error {
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS balances_live, movements, accounts, ledger_day CASCADE;
		DROP FUNCTION IF EXISTS business_day(); DROP FUNCTION IF EXISTS business_prev_day()`); err != nil {
		return fmt.Errorf("drop tables: %w", err)
	}
	if _, err := pool.Exec(ctx, schemaDDL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// Insert accounts: 1 equity, 1 expense:interest, M savings.
	if _, err := pool.Exec(ctx,
		`INSERT INTO accounts (full_path, account_type, product, currency, exponent, annual_interest_rate)
		 VALUES ('Equity:Capital', 'Equity', 'Capital', 'GBP', -2, 0)`); err != nil {
		return fmt.Errorf("insert equity: %w", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO accounts (full_path, account_type, product, currency, exponent, annual_interest_rate)
		 VALUES ('Expense:Interest', 'Expense', 'Interest', 'GBP', -2, 0)`); err != nil {
		return fmt.Errorf("insert expense: %w", err)
	}
	for i := range m {
		path := fmt.Sprintf("Asset:Savings:%04d", i)
		if _, err := pool.Exec(ctx,
			`INSERT INTO accounts (full_path, account_type, product, currency, exponent, annual_interest_rate)
			 VALUES ($1, 'Asset', 'Savings', 'GBP', -2, $2)`,
			path, annualRate); err != nil {
			return fmt.Errorf("insert savings %d: %w", i, err)
		}
	}

	// Seed movements: N per savings account from equity, via CopyFrom.
	fmt.Printf("    Loading %s movements...", benchutil.FmtInt(n*m))
	start := time.Now()

	baseTime := baseDay.Add(12 * time.Hour)
	const batchSize = 10_000
	batch := make([][]any, 0, batchSize)
	batchID := 1

	for acctIdx := range m {
		acctID := firstSavingsID + acctIdx
		for j := range n {
			vt := baseTime.Add(time.Duration(j) * time.Hour)
			batch = append(batch, []any{batchID, equityID, acctID, int64(1000), "PMNT:RCDT:BOOK", int32(0), int64(0), int64(0), vt, time.Now(), "seed"})
			batchID++

			if len(batch) >= batchSize {
				if err := copyMovements(ctx, pool, batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}
	}
	if len(batch) > 0 {
		if err := copyMovements(ctx, pool, batch); err != nil {
			return err
		}
	}

	// One position per savings account per seeded day: the balance through
	// that day's last movement, so every approach reads over the same depth.
	days := seededDays(n)
	positions := make([][]any, 0, batchSize)
	for acctIdx := range m {
		acctID := firstSavingsID + acctIdx
		for d := 0; d < days; d++ {
			day := baseDay.AddDate(0, 0, d)
			positions = append(positions, []any{acctID, day, day.AddDate(0, 0, 1), int64(seedAmount) * int64(movementsThrough(n, d))})
			if len(positions) >= batchSize {
				if err := copyPositions(ctx, pool, positions); err != nil {
					return err
				}
				positions = positions[:0]
			}
		}
	}
	if len(positions) > 0 {
		if err := copyPositions(ctx, pool, positions); err != nil {
			return err
		}
	}
	today := businessDay(n)
	if _, err := pool.Exec(ctx, `INSERT INTO ledger_day (day, prev_day) VALUES ($1, $2)`, today, today.AddDate(0, 0, -1)); err != nil {
		return fmt.Errorf("insert ledger_day: %w", err)
	}

	if _, err := pool.Exec(ctx, "ANALYZE accounts; ANALYZE movements; ANALYZE balances_live; ANALYZE ledger_day"); err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	fmt.Printf(" done (%s, %s positions over %d days)\n", time.Since(start).Round(time.Millisecond), benchutil.FmtInt(days*m), days)
	return nil
}

// Seed movements fall one an hour from noon on baseDay, so N of them span
// seededDays(N) days and movementsThrough(N, d) have happened by the end
// of day d. The business day is the day after the last of them.
var baseDay = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func seededDays(n int) int { return (12+n-1)/24 + 1 }

func movementsThrough(n, d int) int { return min(n, 24*d+12) }

func businessDay(n int) time.Time { return baseDay.AddDate(0, 0, seededDays(n)) }

func copyPositions(ctx context.Context, pool *pgxpool.Pool, rows [][]any) error {
	_, err := pool.CopyFrom(ctx, pgx.Identifier{"balances_live"},
		[]string{"account_id", "balance_date", "next_day", "balance"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("COPY balances_live: %w", err)
	}
	return nil
}

// scanAll runs a whole-ledger read and drains it.
func scanAll(ctx context.Context, pool *pgxpool.Pool, query string) error {
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no rows")
	}
	return nil
}

func copyMovements(ctx context.Context, pool *pgxpool.Pool, rows [][]any) error {
	_, err := pool.CopyFrom(ctx, pgx.Identifier{"movements"},
		[]string{"batch_id", "from_account_id", "to_account_id", "amount", "code", "ledger", "pending_id", "user_data_64", "value_time", "knowledge_time", "description"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("COPY movements: %w", err)
	}
	return nil
}

func runSimple(ctx context.Context, pool *pgxpool.Pool, acctID int, vt time.Time) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var batchID int
	if err := tx.QueryRow(ctx, "SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements").Scan(&batchID); err != nil {
		return fmt.Errorf("batch_id: %w", err)
	}

	var movID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, code, value_time, description)
		 VALUES ($1, $2, $3, $4, 0, $5, 'deposit')
		 RETURNING id`,
		batchID, equityID, acctID, int64(1000), vt).Scan(&movID); err != nil {
		return fmt.Errorf("insert: %w", err)
	}

	var balance int64
	if err := tx.QueryRow(ctx,
		`SELECT
			COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1), 0)
		  - COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $1), 0)`,
		acctID).Scan(&balance); err != nil {
		return fmt.Errorf("balance: %w", err)
	}

	return tx.Commit(ctx)
}

func runCompound(ctx context.Context, pool *pgxpool.Pool, acctID int, vt time.Time) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var batchID int
	if err := tx.QueryRow(ctx, "SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements").Scan(&batchID); err != nil {
		return fmt.Errorf("batch_id: %w", err)
	}

	var movID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, code, value_time, description)
		 VALUES ($1, $2, $3, $4, 0, $5, 'deposit')
		 RETURNING id`,
		batchID, equityID, acctID, int64(1000), vt).Scan(&movID); err != nil {
		return fmt.Errorf("insert: %w", err)
	}

	// End-of-day balance.
	eod := time.Date(vt.Year(), vt.Month(), vt.Day(), 23, 59, 59, 999999999, vt.Location())
	var balance int64
	if err := tx.QueryRow(ctx,
		`SELECT
			COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1 AND value_time <= $2), 0)
		  - COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $1 AND value_time <= $2), 0)`,
		acctID, eod).Scan(&balance); err != nil {
		return fmt.Errorf("eod balance: %w", err)
	}

	// Compute interest via shopspring/decimal.
	balDec := decimal.New(balance, int32(exponent))
	rate := decimal.NewFromFloat(annualRate)
	dailyRate := rate.Div(decimal.NewFromInt(365))
	interestDec := balDec.Mul(dailyRate)
	interest := interestDec.Shift(int32(-exponent)).IntPart()

	bod := time.Date(vt.Year(), vt.Month(), vt.Day(), 0, 0, 0, 0, vt.Location())
	accrualTime := time.Date(vt.Year(), vt.Month(), vt.Day(), 23, 59, 59, 0, vt.Location())

	if interest > 0 {
		// Delete old accrual for this account+day.
		if _, err := tx.Exec(ctx,
			`DELETE FROM movements WHERE to_account_id = $1 AND code = 'LDAS:FTDP:INTR'
			 AND value_time >= $2 AND value_time <= $3`,
			acctID, bod, eod); err != nil {
			return fmt.Errorf("delete accrual: %w", err)
		}

		// Insert new accrual.
		if _, err := tx.Exec(ctx,
			`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, code, value_time, description)
			 VALUES ($1, $2, $3, $4, 'LDAS:FTDP:INTR', $5, 'interest accrual')`,
			batchID, expenseInterestID, acctID, interest, accrualTime); err != nil {
			return fmt.Errorf("insert accrual: %w", err)
		}
	}

	// Delete old live balance.
	if _, err := tx.Exec(ctx,
		`DELETE FROM balances_live WHERE account_id = $1 AND balance_date = $2`,
		acctID, bod); err != nil {
		return fmt.Errorf("delete live: %w", err)
	}

	// Recompute balance after interest.
	var finalBalance int64
	if err := tx.QueryRow(ctx,
		`SELECT
			COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1 AND value_time <= $2), 0)
		  - COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $1 AND value_time <= $2), 0)`,
		acctID, eod).Scan(&finalBalance); err != nil {
		return fmt.Errorf("final balance: %w", err)
	}

	// Insert live balance.
	if _, err := tx.Exec(ctx,
		`INSERT INTO balances_live (account_id, balance_date, next_day, balance)
		 VALUES ($1, $2, $3, $4)`,
		acctID, bod, bod.AddDate(0, 0, 1), finalBalance); err != nil {
		return fmt.Errorf("insert live: %w", err)
	}

	return tx.Commit(ctx)
}

// runPosition is go-luca's RecordMovementWithProjections on a day with no
// later positions: the movement, then today's position rebuilt from the
// previous one plus the day's movements, then the live read.
func runPosition(ctx context.Context, pool *pgxpool.Pool, acctID int, vt time.Time) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var batchID int
	if err := tx.QueryRow(ctx, "SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements").Scan(&batchID); err != nil {
		return fmt.Errorf("batch_id: %w", err)
	}

	var movID int64
	if err := tx.QueryRow(ctx,
		`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, code, value_time, description)
		 VALUES ($1, $2, $3, $4, 0, $5, 'deposit')
		 RETURNING id`,
		batchID, equityID, acctID, int64(1000), vt).Scan(&movID); err != nil {
		return fmt.Errorf("insert: %w", err)
	}

	day := time.Date(vt.Year(), vt.Month(), vt.Day(), 0, 0, 0, 0, vt.Location())
	next := day.AddDate(0, 0, 1)
	var previous int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE((SELECT balance FROM balances_live WHERE account_id = $1 AND balance_date < $2
		                  ORDER BY balance_date DESC LIMIT 1), 0)`,
		acctID, day).Scan(&previous); err != nil {
		return fmt.Errorf("previous position: %w", err)
	}
	var delta int64
	if err := tx.QueryRow(ctx,
		`SELECT
			COALESCE((SELECT SUM(amount) FROM movements WHERE to_account_id = $1 AND value_time >= $2 AND value_time < $3), 0)
		  - COALESCE((SELECT SUM(amount) FROM movements WHERE from_account_id = $1 AND value_time >= $2 AND value_time < $3), 0)`,
		acctID, day, next).Scan(&delta); err != nil {
		return fmt.Errorf("day delta: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO balances_live (account_id, balance_date, next_day, balance)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (account_id, balance_date) DO UPDATE SET balance = EXCLUDED.balance`,
		acctID, day, next, previous+delta); err != nil {
		return fmt.Errorf("upsert position: %w", err)
	}

	var live int64
	if err := tx.QueryRow(ctx, `SELECT balance FROM live_positions WHERE account_id = $1`, acctID).Scan(&live); err != nil {
		return fmt.Errorf("live read: %w", err)
	}
	if live != previous+delta {
		return fmt.Errorf("live read %d, position %d", live, previous+delta)
	}
	return tx.Commit(ctx)
}

func fmtDur(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fus", float64(d)/float64(time.Microsecond))
	case d < time.Second:
		return fmt.Sprintf("%.2fms", float64(d)/float64(time.Millisecond))
	default:
		return fmt.Sprintf("%.3fs", float64(d)/float64(time.Second))
	}
}
