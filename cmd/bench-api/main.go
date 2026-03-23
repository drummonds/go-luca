package main

import (
	"fmt"
	"log"
	"net/http/httptest"
	"path/filepath"
	"time"

	"codeberg.org/hum3/go-luca"
	"codeberg.org/hum3/go-luca/api"
	"codeberg.org/hum3/go-luca/internal/benchutil"
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
		return l, func() { _ = l.Close() }
	}},
	{"api", func() (luca.Ledger, func()) {
		l, err := luca.NewLedger(":memory:")
		if err != nil {
			log.Fatalf("NewLedger: %v", err)
		}
		srv := api.NewServer(l)
		ts := httptest.NewServer(srv)
		client := api.NewClient(ts.URL)
		return client, func() { ts.Close(); _ = l.Close() }
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

	benchDir := "benchmarks/ledger-backends"
	report.AddFileSection("Purpose", filepath.Join(benchDir, "purpose.md"))
	report.AddFileSection("Analysis", filepath.Join(benchDir, "analysis.md"))

	path, err := report.Write("ledger-backends")
	if err != nil {
		log.Fatalf("write report: %v", err)
	}
	fmt.Printf("\nReport written to: %s\n", path)
}

func benchRecordMovement(label string, accountCount int, setup func() (luca.Ledger, func())) (*benchutil.TimingResult, error) {
	l, cleanup := setup()
	defer cleanup()

	from, err := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		log.Fatal(err)
	}
	to, err := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		log.Fatal(err)
	}
	for i := 2; i < accountCount; i++ {
		if _, err := l.CreateAccount(fmt.Sprintf("Asset:Acct%04d", i), "GBP", -2, 0); err != nil {
			log.Fatal(err)
		}
	}

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	i := 0

	return benchutil.RunTimed(label, 0, accountCount, 0, func() error {
		_, err := l.RecordMovement(from.ID, to.ID, 10000, luca.CodeBookTransfer, now.Add(time.Duration(i)*time.Minute), "bench")
		i++
		return err
	})
}

func benchBalance(label string, accountCount, movementCount int, setup func() (luca.Ledger, func())) (*benchutil.TimingResult, error) {
	l, cleanup := setup()
	defer cleanup()

	from, err := l.CreateAccount("Asset:Cash", "GBP", -2, 0)
	if err != nil {
		log.Fatal(err)
	}
	to, err := l.CreateAccount("Equity:Capital", "GBP", -2, 0)
	if err != nil {
		log.Fatal(err)
	}
	for i := 2; i < accountCount; i++ {
		if _, err := l.CreateAccount(fmt.Sprintf("Asset:Acct%04d", i), "GBP", -2, 0); err != nil {
			log.Fatal(err)
		}
	}

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := range movementCount {
		if _, err := l.RecordMovement(from.ID, to.ID, 100, luca.CodeBookTransfer, now.Add(time.Duration(i)*time.Hour), ""); err != nil {
			log.Fatalf("RecordMovement: %v", err)
		}
	}

	return benchutil.RunTimed(label, movementCount, accountCount, 0, func() error {
		_, err := l.Balance(to.ID)
		return err
	})
}
