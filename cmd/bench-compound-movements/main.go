package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/drummonds/go-luca/internal/benchutil"
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
    code SMALLINT NOT NULL DEFAULT 0,
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
    balance BIGINT NOT NULL,
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_balances_live_unique
    ON balances_live(account_id, balance_date);
`

type scenario struct {
	n int // seed movements per savings account
	m int // number of savings accounts
}

var scenarios = []scenario{
	{100, 10},     // 1K movements
	{1_000, 100},  // 100K movements
	{10_000, 100}, // 1M movements
}

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
		"Compare simple vs compound movement throughput on real PostgreSQL")
	report.AddDBInfo(pg.DSN, pg.IsContainer())

	report.AddSQL("Simple operation (single tx)", `BEGIN;
SELECT COALESCE(MAX(batch_id), 0) + 1 FROM movements;
INSERT INTO movements (batch_id, from_account_id, to_account_id, amount,
    code, value_time, description)
  VALUES ($1, $2, $3, $4, 0, $5, 'deposit')
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
DELETE FROM movements WHERE to_account_id=$1 AND code=1
  AND value_time >= $2 AND value_time <= $3;           -- old accrual
INSERT INTO movements (...) VALUES (...);              -- new accrual
DELETE FROM balances_live WHERE account_id=$1
  AND balance_date=$2;                                 -- old live
SELECT ... SUM ... WHERE value_time <= eod;            -- recompute
INSERT INTO balances_live (...) VALUES (...);           -- new live
COMMIT;`)

	report.AddMethods(
		"- **Approaches:** Simple (insert + balance) vs Compound (insert + interest projection + live balance)\n" +
			"- **Schema:** Same as go-luca schema.go (accounts, movements, balances_live)\n" +
			"- **N:** Seed movements per savings account\n" +
			"- **M:** Number of savings accounts (plus 1 equity + 1 expense:interest)\n" +
			"- **Seed data:** N movements per savings account from equity, loaded via pgx CopyFrom in 10K-row batches\n" +
			"- **Interest:** 5% annual rate, exponent -2, computed via shopspring/decimal\n" +
			"- **Iteration target:** Round-robin savings accounts, unique value_time (same day, different minutes)\n" +
			"- **Timing:** Per-iteration wall-clock via benchutil.RunTimed\n" +
			"- **Warmup:** None — first iteration included\n" +
			"- **Transaction:** Both simple and compound wrapped in explicit pgx transactions")

	analysisDir := "benchmarks/analysis"

	for _, sc := range scenarios {
		totalMov := sc.n * sc.m
		fmt.Printf("\n=== N=%s mvts/acct, M=%s accounts (%s total movements) ===\n",
			benchutil.FmtInt(sc.n), benchutil.FmtInt(sc.m), benchutil.FmtInt(totalMov))

		var results []*benchutil.TimingResult

		// --- Simple benchmark ---
		fmt.Printf("  Seeding for simple benchmark...\n")
		if err := resetAndSeed(ctx, pg.Pool, sc.n, sc.m); err != nil {
			log.Fatalf("seed simple: %v", err)
		}

		day := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
		iter := 0
		r, err := benchutil.RunTimed("simple", sc.n, sc.m, 0, func() error {
			acctID := firstSavingsID + (iter % sc.m)
			vt := day.Add(time.Duration(iter) * time.Minute)
			iter++
			return runSimple(ctx, pg.Pool, acctID, vt)
		})
		if err != nil {
			log.Fatalf("bench simple: %v", err)
		}
		fmt.Printf("  simple:   mean=%-10s p50=%-10s p99=%s  TPS=%s\n",
			fmtDur(r.Mean), fmtDur(r.P50), fmtDur(r.P99), benchutil.FmtInt(int(time.Second/r.Mean)))
		results = append(results, r)

		// --- Compound benchmark ---
		fmt.Printf("  Seeding for compound benchmark...\n")
		if err := resetAndSeed(ctx, pg.Pool, sc.n, sc.m); err != nil {
			log.Fatalf("seed compound: %v", err)
		}

		iter = 0
		r, err = benchutil.RunTimed("compound", sc.n, sc.m, 0, func() error {
			acctID := firstSavingsID + (iter % sc.m)
			vt := day.Add(time.Duration(iter) * time.Minute)
			iter++
			return runCompound(ctx, pg.Pool, acctID, vt)
		})
		if err != nil {
			log.Fatalf("bench compound: %v", err)
		}
		fmt.Printf("  compound: mean=%-10s p50=%-10s p99=%s  TPS=%s\n",
			fmtDur(r.Mean), fmtDur(r.P50), fmtDur(r.P99), benchutil.FmtInt(int(time.Second/r.Mean)))
		results = append(results, r)

		report.AddTPSResults(
			fmt.Sprintf("N=%s, M=%s", benchutil.FmtInt(sc.n), benchutil.FmtInt(sc.m)),
			results)
	}

	report.AddFileSection("Purpose", filepath.Join(analysisDir, "compound-movements-purpose.md"))
	report.AddFileSection("Analysis", filepath.Join(analysisDir, "compound-movements-analysis.md"))
	report.AddFileSection("AI Summary", filepath.Join(analysisDir, "compound-movements-ai-summary.md"))

	path, err := report.Write()
	if err != nil {
		log.Fatalf("write report: %v", err)
	}
	fmt.Printf("\nReport written to: %s\n", path)
}

func resetAndSeed(ctx context.Context, pool *pgxpool.Pool, n, m int) error {
	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS balances_live, movements, accounts CASCADE"); err != nil {
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

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	const batchSize = 10_000
	batch := make([][]any, 0, batchSize)
	batchID := 1

	for acctIdx := range m {
		acctID := firstSavingsID + acctIdx
		for j := range n {
			vt := baseTime.Add(time.Duration(j) * time.Hour)
			batch = append(batch, []any{batchID, equityID, acctID, int64(1000), int16(0), int32(0), int64(0), int64(0), vt, time.Now(), "seed"})
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

	if _, err := pool.Exec(ctx, "ANALYZE accounts; ANALYZE movements; ANALYZE balances_live"); err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	fmt.Printf(" done (%s)\n", time.Since(start).Round(time.Millisecond))
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
	defer tx.Rollback(ctx)

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
	defer tx.Rollback(ctx)

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
			`DELETE FROM movements WHERE to_account_id = $1 AND code = 1
			 AND value_time >= $2 AND value_time <= $3`,
			acctID, bod, eod); err != nil {
			return fmt.Errorf("delete accrual: %w", err)
		}

		// Insert new accrual.
		if _, err := tx.Exec(ctx,
			`INSERT INTO movements (batch_id, from_account_id, to_account_id, amount, code, value_time, description)
			 VALUES ($1, $2, $3, $4, 1, $5, 'interest accrual')`,
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
		`INSERT INTO balances_live (account_id, balance_date, balance)
		 VALUES ($1, $2, $3)`,
		acctID, bod, finalBalance); err != nil {
		return fmt.Errorf("insert live: %w", err)
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
