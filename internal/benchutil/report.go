package benchutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Report builds a markdown benchmark report.
type Report struct {
	Title    string
	Purpose  string
	sections []string
}

// NewReport creates a new benchmark report.
func NewReport(title, purpose string) *Report {
	return &Report{Title: title, Purpose: purpose}
}

// AddDBInfo adds a database information section.
func (r *Report) AddDBInfo(dsn string, isContainer bool) {
	source := "external (BENCH_PG_DSN)"
	if isContainer {
		source = "podman postgres:16-alpine"
	}
	r.sections = append(r.sections, fmt.Sprintf("## Database\n\n- Source: %s\n- DSN: `%s`\n\n", source, redactDSN(dsn)))
}

// AddSQL adds a sample SQL section.
func (r *Report) AddSQL(label, sql string) {
	r.sections = append(r.sections, fmt.Sprintf("## SQL: %s\n\n```sql\n%s\n```\n\n", label, sql))
}

// AddResults adds a timing results table.
func (r *Report) AddResults(label string, results []*TimingResult) {
	var b strings.Builder
	fmt.Fprintf(&b, "## Results: %s\n\n", label)
	fmt.Fprintln(&b, "| Type | N | M | Mean | P50 | P99 | Min | Max | Iters |")
	fmt.Fprintln(&b, "|------|---|---|------|-----|-----|-----|-----|-------|")
	for _, t := range results {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %d |\n",
			t.Label, FmtInt(t.N), FmtInt(t.M),
			fmtDur(t.Mean), fmtDur(t.P50), fmtDur(t.P99),
			fmtDur(t.Min), fmtDur(t.Max), t.Iterations)
	}
	b.WriteString("\n")
	r.sections = append(r.sections, b.String())
}

// AddTPSResults adds a timing results table with a TPS column computed from Mean.
func (r *Report) AddTPSResults(label string, results []*TimingResult) {
	var b strings.Builder
	fmt.Fprintf(&b, "## Results: %s\n\n", label)
	fmt.Fprintln(&b, "| Approach | N | M | Mean | TPS | P50 | P99 | Min | Max | Iters |")
	fmt.Fprintln(&b, "|----------|---|---|------|-----|-----|-----|-----|-----|-------|")
	for _, t := range results {
		tps := "—"
		if t.Mean > 0 {
			tps = FmtInt(int(time.Second / t.Mean))
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %d |\n",
			t.Label, FmtInt(t.N), FmtInt(t.M),
			fmtDur(t.Mean), tps, fmtDur(t.P50), fmtDur(t.P99),
			fmtDur(t.Min), fmtDur(t.Max), t.Iterations)
	}
	b.WriteString("\n")
	r.sections = append(r.sections, b.String())
}

// AddMethods adds a methodology section.
func (r *Report) AddMethods(text string) {
	r.sections = append(r.sections, fmt.Sprintf("## Methods\n\n%s\n\n", text))
}

// AddFileSection reads a markdown file and appends it under the given heading.
// If the file doesn't exist, adds a placeholder.
func (r *Report) AddFileSection(heading, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		r.sections = append(r.sections, fmt.Sprintf("## %s\n\n_No file found: %s_\n\n", heading, path))
		return
	}
	r.sections = append(r.sections, fmt.Sprintf("## %s\n\n%s\n\n", heading, strings.TrimSpace(string(data))))
}

// Write outputs the report to benchmarks/reports/<slug>.md, overwriting any previous run.
// Returns the file path written.
func (r *Report) Write() (string, error) {
	slug := strings.ReplaceAll(strings.ToLower(r.Title), " ", "-")
	filename := fmt.Sprintf("%s.md", slug)

	dir := filepath.Join("benchmarks", "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, filename)

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", r.Title)
	fmt.Fprintf(&b, "**Date:** %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "**Purpose:** %s\n\n", r.Purpose)

	for _, s := range r.sections {
		b.WriteString(s)
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// FmtInt formats an integer with _ separators (e.g. 1000000 → 1_000_000).
func FmtInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	offset := len(s) % 3
	if offset > 0 {
		b.WriteString(s[:offset])
	}
	for i := offset; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte('_')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
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

func redactDSN(dsn string) string {
	// Simple password redaction: replace password between : and @ after ://
	if idx := strings.Index(dsn, "://"); idx >= 0 {
		rest := dsn[idx+3:]
		if atIdx := strings.Index(rest, "@"); atIdx >= 0 {
			userPass := rest[:atIdx]
			if colonIdx := strings.Index(userPass, ":"); colonIdx >= 0 {
				return dsn[:idx+3] + userPass[:colonIdx] + ":***@" + rest[atIdx+1:]
			}
		}
	}
	return dsn
}
