package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/drummonds/go-luca/internal/benchutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
`

var products = []string{"IAA", "ISA", "NA", "FD1"}

type scenario struct {
	accounts   int
	iterations int
}

var scenarios = []scenario{
	{1_000, 50},
	{10_000, 20},
	{100_000, 5},
	{1_000_000, 1},
}

const (
	equityID        = 1
	fscsFooter      = "99999999999999999999"
	compensationCap = 8_500_000 // £85,000 in pence
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

	report := benchutil.NewReport("SCV Generation",
		"Benchmark FSCS Single Customer View file generation at various account scales")
	report.AddDBInfo(pg.DSN, pg.IsContainer())

	report.AddSQL("SCV balance query", `WITH credits AS (
    SELECT to_account_id AS account_id, SUM(amount) AS total
    FROM movements GROUP BY to_account_id
),
debits AS (
    SELECT from_account_id AS account_id, SUM(amount) AS total
    FROM movements GROUP BY from_account_id
)
SELECT a.id, a.account_id, a.product, a.currency,
    COALESCE(c.total, 0) - COALESCE(d.total, 0) AS balance
FROM accounts a
LEFT JOIN credits c ON c.account_id = a.id
LEFT JOIN debits d ON d.account_id = a.id
WHERE a.account_type = 'Liability'`)

	report.AddMethods(
		"- **Scenarios:** 1K, 10K, 100K, 1M liability accounts\n" +
			"- **Schema:** go-luca (accounts + movements), customers modelled via account_id field\n" +
			"- **Products:** Random from IAA, ISA, NA, FD1\n" +
			"- **Balances:** One seed movement per account (0\u2013100K GBP random)\n" +
			"- **Iteration:** Full SCV generation: query \u2192 group \u2192 write C file \u2192 write D file\n" +
			"- **C file:** Pipe-delimited individual account records per FSCS spec\n" +
			"- **D file:** Pipe-delimited per-customer aggregate, compensatable capped at \u00a385,000\n" +
			"- **Footer:** FSCS standard `99999999999999999999`")

	benchDir := "benchmarks/scv"
	tmpDir := os.TempDir()

	for _, sc := range scenarios {
		fmt.Printf("\n=== %s accounts ===\n", benchutil.FmtInt(sc.accounts))

		fmt.Println("  Seeding...")
		numCust, err := resetAndSeed(ctx, pg.Pool, sc.accounts)
		if err != nil {
			log.Fatalf("seed: %v", err)
		}
		fmt.Printf("  %s customers, %s accounts\n",
			benchutil.FmtInt(numCust), benchutil.FmtInt(sc.accounts))

		cPath := filepath.Join(tmpDir, fmt.Sprintf("scv-c-%d.csv", sc.accounts))
		dPath := filepath.Join(tmpDir, fmt.Sprintf("scv-d-%d.csv", sc.accounts))

		r, err := benchutil.RunTimed("scv", sc.accounts, numCust, sc.iterations, func() error {
			return generateSCV(ctx, pg.Pool, cPath, dPath)
		})
		if err != nil {
			log.Fatalf("bench: %v", err)
		}
		fmt.Printf("  mean=%-10s p50=%-10s p99=%s\n",
			fmtDur(r.Mean), fmtDur(r.P50), fmtDur(r.P99))

		report.AddResults(
			fmt.Sprintf("%s accounts, %s customers",
				benchutil.FmtInt(sc.accounts), benchutil.FmtInt(numCust)),
			[]*benchutil.TimingResult{r})

		os.Remove(cPath)
		os.Remove(dPath)
	}

	report.AddFileSection("Purpose", filepath.Join(benchDir, "purpose.md"))
	report.AddFileSection("Analysis", filepath.Join(benchDir, "analysis.md"))

	path, err := report.Write("scv")
	if err != nil {
		log.Fatalf("write report: %v", err)
	}
	fmt.Printf("\nReport written to: %s\n", path)
}

func resetAndSeed(ctx context.Context, pool *pgxpool.Pool, numAccounts int) (int, error) {
	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS balances_live, movements, accounts CASCADE"); err != nil {
		return 0, fmt.Errorf("drop: %w", err)
	}
	if _, err := pool.Exec(ctx, schemaDDL); err != nil {
		return 0, fmt.Errorf("schema: %w", err)
	}

	// Equity account (id=1).
	if _, err := pool.Exec(ctx,
		`INSERT INTO accounts (full_path, account_type, product, currency, exponent)
		 VALUES ('Equity:Capital', 'Equity', 'Capital', 'GBP', -2)`); err != nil {
		return 0, fmt.Errorf("equity: %w", err)
	}

	rng := rand.New(rand.NewSource(42))

	// Assign accounts to customers (1-100 accounts each).
	type acctInfo struct {
		customerID string
		product    string
	}
	infos := make([]acctInfo, 0, numAccounts)
	custNum := 0
	for len(infos) < numAccounts {
		custNum++
		custID := fmt.Sprintf("CUST%06d", custNum)
		n := 1 + rng.Intn(100)
		if len(infos)+n > numAccounts {
			n = numAccounts - len(infos)
		}
		for range n {
			infos = append(infos, acctInfo{
				customerID: custID,
				product:    products[rng.Intn(len(products))],
			})
		}
	}
	numCustomers := custNum

	// Bulk insert accounts.
	fmt.Printf("    Loading %s accounts...", benchutil.FmtInt(numAccounts))
	start := time.Now()

	const batchSize = 10_000
	acctBatch := make([][]any, 0, batchSize)
	for i, info := range infos {
		path := fmt.Sprintf("Liability:%s:%s:%06d", info.product, info.customerID, i)
		acctBatch = append(acctBatch, []any{
			path, "Liability", info.product, info.customerID, "", false, "GBP", -2, 0.0,
		})
		if len(acctBatch) >= batchSize {
			if err := copyAccounts(ctx, pool, acctBatch); err != nil {
				return 0, err
			}
			acctBatch = acctBatch[:0]
		}
	}
	if len(acctBatch) > 0 {
		if err := copyAccounts(ctx, pool, acctBatch); err != nil {
			return 0, err
		}
	}
	fmt.Printf(" done (%s)\n", time.Since(start).Round(time.Millisecond))

	// Bulk insert movements (one per account from equity).
	fmt.Printf("    Loading %s movements...", benchutil.FmtInt(numAccounts))
	start = time.Now()

	baseTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	movBatch := make([][]any, 0, batchSize)
	for i := range numAccounts {
		acctID := i + 2 // equity=1, liability accounts start at 2
		amount := int64(rng.Intn(10_000_001))
		vt := baseTime.Add(time.Duration(i) * time.Second)
		movBatch = append(movBatch, []any{
			1, equityID, acctID, amount, int16(0), int32(0), int64(0), int64(0), vt, time.Now(), "seed",
		})
		if len(movBatch) >= batchSize {
			if err := copyMovements(ctx, pool, movBatch); err != nil {
				return 0, err
			}
			movBatch = movBatch[:0]
		}
	}
	if len(movBatch) > 0 {
		if err := copyMovements(ctx, pool, movBatch); err != nil {
			return 0, err
		}
	}

	if _, err := pool.Exec(ctx, "ANALYZE accounts; ANALYZE movements"); err != nil {
		return 0, fmt.Errorf("analyze: %w", err)
	}
	fmt.Printf(" done (%s)\n", time.Since(start).Round(time.Millisecond))
	return numCustomers, nil
}

func copyAccounts(ctx context.Context, pool *pgxpool.Pool, rows [][]any) error {
	_, err := pool.CopyFrom(ctx, pgx.Identifier{"accounts"},
		[]string{"full_path", "account_type", "product", "account_id", "address", "is_pending", "currency", "exponent", "annual_interest_rate"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("COPY accounts: %w", err)
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

type acctRow struct {
	id       int64
	custID   string
	product  string
	currency string
	balance  int64
}

func generateSCV(ctx context.Context, pool *pgxpool.Pool, cPath, dPath string) error {
	rows, err := pool.Query(ctx, `
		WITH credits AS (
			SELECT to_account_id AS account_id, SUM(amount) AS total
			FROM movements GROUP BY to_account_id
		),
		debits AS (
			SELECT from_account_id AS account_id, SUM(amount) AS total
			FROM movements GROUP BY from_account_id
		)
		SELECT a.id, a.account_id, a.product, a.currency,
			COALESCE(c.total, 0) - COALESCE(d.total, 0) AS balance
		FROM accounts a
		LEFT JOIN credits c ON c.account_id = a.id
		LEFT JOIN debits d ON d.account_id = a.id
		WHERE a.account_type = 'Liability'`)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	// Group by customer.
	type customerData struct {
		accounts []acctRow
		total    int64
	}
	customers := make(map[string]*customerData)
	var order []string

	for rows.Next() {
		var r acctRow
		if err := rows.Scan(&r.id, &r.custID, &r.product, &r.currency, &r.balance); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		cd, ok := customers[r.custID]
		if !ok {
			cd = &customerData{}
			customers[r.custID] = cd
			order = append(order, r.custID)
		}
		cd.accounts = append(cd.accounts, r)
		cd.total += r.balance
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	// Write C file (individual accounts).
	cFile, err := os.Create(cPath)
	if err != nil {
		return err
	}
	defer cFile.Close()
	cBuf := bufio.NewWriter(cFile)

	// Write D file (customer aggregates).
	dFile, err := os.Create(dPath)
	if err != nil {
		return err
	}
	defer dFile.Close()
	dBuf := bufio.NewWriter(dFile)

	recNum := 1
	for _, custID := range order {
		cd := customers[custID]
		for _, a := range cd.accounts {
			bal := formatPence(a.balance)
			fmt.Fprintf(cBuf, "%d|%s|%d|000000|%s|1|A|0|UK|N|N|%s|0|%s|%s|1.000000|%s|Y\n",
				recNum, custID, a.id, a.product, bal, a.currency, bal, bal)
		}

		aggBal := formatPence(cd.total)
		comp := cd.total
		if comp > compensationCap {
			comp = compensationCap
		}
		fmt.Fprintf(dBuf, "%d|%s|%s\n", recNum, aggBal, formatPence(comp))
		recNum++
	}

	// FSCS footer.
	fmt.Fprintln(cBuf, fscsFooter)
	fmt.Fprintln(dBuf, fscsFooter)

	if err := cBuf.Flush(); err != nil {
		return fmt.Errorf("flush C: %w", err)
	}
	if err := dBuf.Flush(); err != nil {
		return fmt.Errorf("flush D: %w", err)
	}
	return nil
}

func formatPence(pence int64) string {
	pounds := pence / 100
	remainder := pence % 100
	if remainder < 0 {
		remainder = -remainder
	}
	return fmt.Sprintf("%d.%02d", pounds, remainder)
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
