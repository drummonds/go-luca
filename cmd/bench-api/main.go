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

// backend describes a Ledger backend to benchmark.
type backend struct {
	name  string
	setup func() (luca.Ledger, func()) // returns ledger + cleanup
}

var backends = []backend{
	{"mem", func() (luca.Ledger, func()) {
		return luca.NewMemLedger(), func() {}
	}},
	{"sql", func() (luca.Ledger, func()) {
		l, err := luca.NewLedger(":memory:")
		if err != nil {
			log.Fatalf("NewLedger: %v", err)
		}
		return l, func() { l.Close() }
	}},
	{"api", func() (luca.Ledger, func()) {
		l, err := luca.NewLedger(":memory:")
		if err != nil {
			log.Fatalf("NewLedger: %v", err)
		}
		srv := api.NewServer(l)
		ts := httptest.NewServer(srv)
		client := api.NewClient(ts.URL)
		return client, func() { ts.Close(); l.Close() }
	}},
}

func main() {
	report := benchutil.NewReport("Ledger Backends",
		"Compare Ledger performance across backends: MemLedger, SQLLedger (pglike), and HTTP/JSON API")
	report.AddMethods(
		"- **mem:** Pure Go in-memory MemLedger\n" +
			"- **sql:** SQLLedger with pglike/SQLite :memory:\n" +
			"- **api:** api.Client → httptest.Server → api.Server → SQLLedger :memory:\n" +
			"- **N:** Number of movements (pre-loaded for balance queries)\n" +
			"- **M:** Number of accounts\n" +
			"- Each scenario creates a fresh ledger to avoid cross-contamination\n" +
			"- Warmup: None — first iteration included")

	scenarios := []struct {
		label    string
		accounts int
		preload  int
	}{
		{"small", 10, 100},
		{"medium", 100, 1000},
		{"large", 1000, 10000},
	}

	// --- RecordMovement TPS ---
	fmt.Println("=== RecordMovement TPS ===")
	for _, sc := range scenarios {
		fmt.Printf("  %s (M=%d)...\n", sc.label, sc.accounts)

		var results []*benchutil.TimingResult
		for _, be := range backends {
			r, err := benchRecordMovement(be.name, sc.accounts, be.setup)
			if err != nil {
				log.Fatal(err)
			}
			results = append(results, r)
		}

		report.AddTPSResults(
			fmt.Sprintf("RecordMovement (%s, M=%s)", sc.label, benchutil.FmtInt(sc.accounts)),
			results)
	}

	// --- Balance query latency ---
	fmt.Println("=== Balance Query Latency ===")
	for _, sc := range scenarios {
		fmt.Printf("  %s (N=%d, M=%d)...\n", sc.label, sc.preload, sc.accounts)

		var results []*benchutil.TimingResult
		for _, be := range backends {
			r, err := benchBalance(be.name, sc.accounts, sc.preload, be.setup)
			if err != nil {
				log.Fatal(err)
			}
			results = append(results, r)
		}

		report.AddResults(
			fmt.Sprintf("Balance query (%s, N=%s, M=%s)", sc.label, benchutil.FmtInt(sc.preload), benchutil.FmtInt(sc.accounts)),
			results)
	}

	analysisDir := "benchmarks/analysis"
	report.AddFileSection("Purpose", filepath.Join(analysisDir, "ledger-backends-purpose.md"))
	report.AddFileSection("Analysis", filepath.Join(analysisDir, "ledger-backends-analysis.md"))

	path, err := report.Write()
	if err != nil {
		log.Fatalf("write report: %v", err)
	}
	fmt.Printf("\nReport written to: %s\n", path)
}

func benchRecordMovement(label string, accountCount int, setup func() (luca.Ledger, func())) (*benchutil.TimingResult, error) {
	l, cleanup := setup()
	defer cleanup()

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

func benchBalance(label string, accountCount, movementCount int, setup func() (luca.Ledger, func())) (*benchutil.TimingResult, error) {
	l, cleanup := setup()
	defer cleanup()

	from, _ := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	to, _ := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	for i := 2; i < accountCount; i++ {
		l.CreateAccount(fmt.Sprintf("Asset:Acct%04d", i), "GBP", -2, 0)
	}

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := range movementCount {
		l.RecordMovement(from.ID, to.ID, 100, now.Add(time.Duration(i)*time.Hour), "")
	}

	return benchutil.RunTimed(label, movementCount, accountCount, 0, func() error {
		_, err := l.Balance(to.ID)
		return err
	})
}
