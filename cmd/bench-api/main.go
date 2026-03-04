package main

import (
	"fmt"
	"log"
	"net/http/httptest"
	"path/filepath"
	"time"

	"github.com/drummonds/go-luca"
	"github.com/drummonds/go-luca/api"
	"github.com/drummonds/go-luca/internal/benchutil"
	_ "github.com/drummonds/go-postgres"
)

func main() {
	report := benchutil.NewReport("Direct vs API",
		"Compare Ledger performance via direct method calls vs HTTP/JSON API round-trips")
	report.AddMethods(
		"- **Direct:** Call Ledger methods directly on *SQLLedger (in-process pglike/SQLite :memory:)\n" +
			"- **API:** Call via api.Client → httptest.Server → api.Server → same *SQLLedger\n" +
			"- **N:** Number of movements (pre-loaded)\n" +
			"- **M:** Number of accounts\n" +
			"- Each scenario creates a fresh ledger to avoid cross-contamination\n" +
			"- Warmup: None — first iteration included")

	scenarios := []struct {
		label    string
		accounts int
		preload  int // movements to pre-load before balance queries
	}{
		{"small", 10, 100},
		{"medium", 100, 1000},
		{"large", 1000, 10000},
	}

	// --- RecordMovement TPS ---
	fmt.Println("=== RecordMovement TPS ===")
	for _, sc := range scenarios {
		fmt.Printf("  %s (M=%d)...\n", sc.label, sc.accounts)

		directResult, err := benchRecordMovement(sc.label+"/direct", sc.accounts, func(l luca.Ledger) luca.Ledger { return l })
		if err != nil {
			log.Fatal(err)
		}
		apiResult, err := benchRecordMovement(sc.label+"/api", sc.accounts, wrapAPI)
		if err != nil {
			log.Fatal(err)
		}

		report.AddTPSResults(
			fmt.Sprintf("RecordMovement (%s, M=%s)", sc.label, benchutil.FmtInt(sc.accounts)),
			[]*benchutil.TimingResult{directResult, apiResult})
	}

	// --- Balance query latency ---
	fmt.Println("=== Balance Query Latency ===")
	for _, sc := range scenarios {
		fmt.Printf("  %s (N=%d, M=%d)...\n", sc.label, sc.preload, sc.accounts)

		directResult, err := benchBalance(sc.label+"/direct", sc.accounts, sc.preload, func(l luca.Ledger) luca.Ledger { return l })
		if err != nil {
			log.Fatal(err)
		}
		apiResult, err := benchBalance(sc.label+"/api", sc.accounts, sc.preload, wrapAPI)
		if err != nil {
			log.Fatal(err)
		}

		report.AddResults(
			fmt.Sprintf("Balance query (%s, N=%s, M=%s)", sc.label, benchutil.FmtInt(sc.preload), benchutil.FmtInt(sc.accounts)),
			[]*benchutil.TimingResult{directResult, apiResult})
	}

	analysisDir := "benchmarks/analysis"
	report.AddFileSection("Purpose", filepath.Join(analysisDir, "direct-vs-api-purpose.md"))
	report.AddFileSection("Analysis", filepath.Join(analysisDir, "direct-vs-api-analysis.md"))

	path, err := report.Write()
	if err != nil {
		log.Fatalf("write report: %v", err)
	}
	fmt.Printf("\nReport written to: %s\n", path)
}

type wrapFn func(luca.Ledger) luca.Ledger

func wrapAPI(l luca.Ledger) luca.Ledger {
	srv := api.NewServer(l)
	ts := httptest.NewServer(srv)
	// Note: ts never closed in benchmarks — acceptable for short-lived process
	return api.NewClient(ts.URL)
}

func benchRecordMovement(label string, accountCount int, wrap wrapFn) (*benchutil.TimingResult, error) {
	backing, err := luca.NewLedger(":memory:")
	if err != nil {
		return nil, err
	}
	defer backing.Close()

	l := wrap(backing)

	// Create accounts
	from, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	to, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	for i := 2; i < accountCount; i++ {
		l.CreateAccount(fmt.Sprintf("Asset:Acct%04d", i), "GBP", -2, 0)
	}

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	i := 0

	return benchutil.RunTimed(label, 0, accountCount, 0, func() error {
		_, err := l.RecordMovement(from.ID, to.ID, 10000, now.Add(time.Duration(i)*time.Minute), "bench")
		i++
		return err
	})
}

func benchBalance(label string, accountCount, movementCount int, wrap wrapFn) (*benchutil.TimingResult, error) {
	backing, err := luca.NewLedger(":memory:")
	if err != nil {
		return nil, err
	}
	defer backing.Close()

	l := wrap(backing)

	from, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	to, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	for i := 2; i < accountCount; i++ {
		// Create via backing to avoid API overhead during setup
		backing.CreateAccount(fmt.Sprintf("Asset:Acct%04d", i), "GBP", -2, 0)
	}

	// Pre-load movements via backing for speed
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := range movementCount {
		backing.RecordMovement(from.ID, to.ID, 100, now.Add(time.Duration(i)*time.Hour), "")
	}

	return benchutil.RunTimed(label, movementCount, accountCount, 0, func() error {
		_, err := l.Balance(to.ID)
		return err
	})
}
