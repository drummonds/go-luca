package benchutil

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"
)

// TimingResult holds statistics for a benchmark run.
type TimingResult struct {
	Label      string
	N          int // row count
	M          int // account count
	Iterations int
	Durations  []time.Duration
	Mean       time.Duration
	Min        time.Duration
	Max        time.Duration
	P50        time.Duration
	P99        time.Duration
}

// RunTimed executes fn for the configured number of iterations and computes stats.
func RunTimed(label string, n, m, iterations int, fn func() error) (*TimingResult, error) {
	if iterations <= 0 {
		iterations = defaultIterations()
	}

	durations := make([]time.Duration, 0, iterations)
	for range iterations {
		start := time.Now()
		if err := fn(); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		durations = append(durations, time.Since(start))
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	var total time.Duration
	for _, d := range durations {
		total += d
	}

	r := &TimingResult{
		Label:      label,
		N:          n,
		M:          m,
		Iterations: iterations,
		Durations:  durations,
		Mean:       total / time.Duration(iterations),
		Min:        durations[0],
		Max:        durations[iterations-1],
		P50:        percentile(durations, 0.50),
		P99:        percentile(durations, 0.99),
	}
	return r, nil
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func defaultIterations() int {
	if s := os.Getenv("BENCH_ITERATIONS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return 100
}
