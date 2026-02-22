package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"path/filepath"
	"time"

	"github.com/drummonds/go-luca/internal/benchutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type colType struct {
	name    string
	sqlType string
}

var types = []colType{
	{"bigint", "BIGINT"},
	{"double", "DOUBLE PRECISION"},
	{"numeric", "NUMERIC(20,7)"},
}

type scenario struct {
	n int // rows per account
	m int // account count
}

var scenarios = []scenario{
	{10, 10_000},
	{2, 1_000_000},
	{30, 1_000_000},
	{365, 100_000},
}

func main() {
	ctx := context.Background()

	fmt.Println("Starting PostgreSQL...")
	pg, err := benchutil.StartPG(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pg.Stop(ctx)
	fmt.Println("PostgreSQL ready.")

	report := benchutil.NewReport("Balance Type Comparison",
		"Compare point-in-time balance lookup performance across BIGINT, DOUBLE PRECISION, and NUMERIC(20,7) data types")
	report.AddDBInfo(pg.DSN, pg.IsContainer())

	lookupSQL := `SELECT value, pending_in, pending_out FROM balances_<type>
  WHERE account_id = $1 AND value_time <= $2
  ORDER BY value_time DESC LIMIT 1`
	report.AddSQL("Point-in-time balance lookup", lookupSQL)

	report.AddMethods("- **Data types tested:** BIGINT (int64), DOUBLE PRECISION (float64), NUMERIC(20,7) (arbitrary precision)\n" +
		"- **Schema:** Each balance row has `value`, `pending_in`, `pending_out` columns\n" +
		"- **N:** Rows per account (balance snapshots over time)\n" +
		"- **M:** Number of accounts. Total rows = N * M\n" +
		"- **Data generation:** Fixed random seed (42), one row per account per time step\n" +
		"- **Bulk load:** pgx CopyFrom in 10K-row batches, ANALYZE after load\n" +
		"- **Warmup:** None — first iteration included (captures cold-cache behaviour)\n" +
		"- **Timing:** Per-iteration wall-clock, statistics over all iterations\n" +
		"- **Query:** Point-in-time lookup — latest balance at or before a given timestamp for one account\n" +
		"\n### Table DDL\n\n" +
		"One table per type per (N, M) scenario:\n\n" +
		"```sql\n" +
		"CREATE TABLE balances_<type>_<N>_<M> (\n" +
		"    id SERIAL PRIMARY KEY,\n" +
		"    account_id INT NOT NULL,\n" +
		"    value_time TIMESTAMP NOT NULL,\n" +
		"    value       <TYPE> NOT NULL,\n" +
		"    pending_in  <TYPE> NOT NULL,\n" +
		"    pending_out <TYPE> NOT NULL\n" +
		");\n" +
		"CREATE INDEX ON balances_<type>_<N>_<M> (account_id, value_time DESC);\n" +
		"```")

	analysisDir := "benchmarks/analysis"

	for _, sc := range scenarios {
		totalRows := sc.n * sc.m
		fmt.Printf("\n=== N=%s rows/acct, M=%s accounts (%s total) ===\n",
			benchutil.FmtInt(sc.n), benchutil.FmtInt(sc.m), benchutil.FmtInt(totalRows))

		// Create tables and load data.
		for _, ct := range types {
			tableName := fmt.Sprintf("balances_%s_%d_%d", ct.name, sc.n, sc.m)
			if err := setupTable(ctx, pg.Pool, tableName, ct.sqlType, sc.n, sc.m); err != nil {
				log.Fatalf("setup %s: %v", tableName, err)
			}
		}

		// Benchmark point-in-time balance lookup.
		var lookupResults []*benchutil.TimingResult
		for _, ct := range types {
			tableName := fmt.Sprintf("balances_%s_%d_%d", ct.name, sc.n, sc.m)
			sql := fmt.Sprintf(
				"SELECT value, pending_in, pending_out FROM %s WHERE account_id = $1 AND value_time <= $2 ORDER BY value_time DESC LIMIT 1",
				tableName)

			// Query a mid-range account at a mid-range time.
			accountID := sc.m / 2
			baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			queryTime := baseTime.AddDate(0, 0, sc.n/2)

			r, err := benchutil.RunTimed(ct.name, sc.n, sc.m, 0, func() error {
				var val, pendIn, pendOut any
				return pg.Pool.QueryRow(ctx, sql, accountID, queryTime).Scan(&val, &pendIn, &pendOut)
			})
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("  %-8s lookup: mean=%-10s p50=%-10s p99=%s\n",
				ct.name, fmtDur(r.Mean), fmtDur(r.P50), fmtDur(r.P99))
			lookupResults = append(lookupResults, r)
		}
		report.AddResults(
			fmt.Sprintf("Point-in-time lookup (N=%s, M=%s)", benchutil.FmtInt(sc.n), benchutil.FmtInt(sc.m)),
			lookupResults)

		// Drop tables to free memory for next scenario.
		for _, ct := range types {
			tableName := fmt.Sprintf("balances_%s_%d_%d", ct.name, sc.n, sc.m)
			pg.Pool.Exec(ctx, "DROP TABLE IF EXISTS "+tableName)
		}
	}

	report.AddFileSection("Purpose", filepath.Join(analysisDir, "balance-types-purpose.md"))
	report.AddFileSection("Analysis", filepath.Join(analysisDir, "balance-types-analysis.md"))
	report.AddFileSection("AI Summary", filepath.Join(analysisDir, "balance-types-ai-summary.md"))

	path, err := report.Write()
	if err != nil {
		log.Fatalf("write report: %v", err)
	}
	fmt.Printf("\nReport written to: %s\n", path)
}

// setupTable creates a balances table and populates it with N rows per account, M accounts.
func setupTable(ctx context.Context, pool *pgxpool.Pool, tableName, sqlType string, n, m int) error {
	ddl := fmt.Sprintf(`DROP TABLE IF EXISTS %s;
CREATE TABLE %s (
	id SERIAL PRIMARY KEY,
	account_id INT NOT NULL,
	value_time TIMESTAMP NOT NULL,
	value %s NOT NULL,
	pending_in %s NOT NULL,
	pending_out %s NOT NULL
);
CREATE INDEX ON %s (account_id, value_time DESC);`, tableName, tableName, sqlType, sqlType, sqlType, tableName)

	if _, err := pool.Exec(ctx, ddl); err != nil {
		return fmt.Errorf("DDL: %w", err)
	}

	totalRows := n * m
	fmt.Printf("  Loading %s (%s rows/acct * %s accounts = %s rows)...",
		tableName, benchutil.FmtInt(n), benchutil.FmtInt(m), benchutil.FmtInt(totalRows))
	start := time.Now()

	rng := rand.New(rand.NewSource(42))
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Generate rows: for each time step, one row per account.
	const batchSize = 10_000
	batch := make([][]any, 0, batchSize)
	for day := range n {
		vt := baseTime.AddDate(0, 0, day)
		for acct := 1; acct <= m; acct++ {
			value := rng.Int63n(1_000_000) - 500_000
			pendIn := rng.Int63n(100_000)
			pendOut := rng.Int63n(100_000)
			batch = append(batch, []any{acct, vt, value, pendIn, pendOut})

			if len(batch) >= batchSize {
				if err := copyBatch(ctx, pool, tableName, batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}
	}
	if len(batch) > 0 {
		if err := copyBatch(ctx, pool, tableName, batch); err != nil {
			return err
		}
	}

	pool.Exec(ctx, "ANALYZE "+tableName)
	fmt.Printf(" done (%s)\n", time.Since(start).Round(time.Millisecond))
	return nil
}

func copyBatch(ctx context.Context, pool *pgxpool.Pool, tableName string, rows [][]any) error {
	_, err := pool.CopyFrom(ctx, pgx.Identifier{tableName},
		[]string{"account_id", "value_time", "value", "pending_in", "pending_out"},
		pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("COPY: %w", err)
	}
	return nil
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
