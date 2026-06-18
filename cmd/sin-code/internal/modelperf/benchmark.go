// SPDX-License-Identifier: MIT
// Purpose: Benchmark runner for the Model Performance Registry (issue #395).
//
// Runs a golden dataset across all fusion providers in parallel, collects
// per-model results (pass/fail, latency, tokens, cost), and records them
// into the modelperf store.
//
// Race-free (M7): sync.WaitGroup + buffered channel.
package modelperf

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
)

// BenchmarkProvider is one model's execution interface for benchmarking.
// The implementation runs a single test case prompt and returns the result.
type BenchmarkProvider interface {
	Name() string
	Run(ctx context.Context, prompt string) (BenchmarkResult, error)
}

// BenchmarkResult is the outcome of running one test case on one model.
type BenchmarkResult struct {
	Passed   bool
	Latency  time.Duration
	Tokens   int
	CostUSD  float64
	Output   string
	Error    string
}

// BenchmarkConfig controls a benchmark run.
type BenchmarkConfig struct {
	DatasetPath string
	Category    string // auto-detected if empty
	Parallel    bool   // run providers in parallel (default true)
	Timeout     time.Duration
}

// BenchmarkOutcome is the aggregate result of a benchmark run.
type BenchmarkOutcome struct {
	Category  string
	Dataset   string
	Providers int
	Cases     int
	Results   []PerProviderResult
}

// PerProviderResult is one model's aggregate result across all test cases.
type PerProviderResult struct {
	Model       string
	PassRate    float64
	AvgLatency  time.Duration
	AvgCost     float64
	AvgTokens   int
	Passed      int
	Total       int
}

// RunBenchmark executes a dataset across all providers and records results
// into the store. Returns the aggregate outcome.
func RunBenchmark(ctx context.Context, store *Store, providers []BenchmarkProvider, cfg BenchmarkConfig) (*BenchmarkOutcome, error) {
	if store == nil {
		return nil, fmt.Errorf("modelperf: store is nil")
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("modelperf: no providers to benchmark")
	}

	// Load dataset
	ds, err := dataset.LoadDataset(cfg.DatasetPath)
	if err != nil {
		return nil, fmt.Errorf("modelperf: parse dataset: %w", err)
	}

	category := cfg.Category
	if category == "" {
		category = DetectCategory(ds.Name + " " + ds.Description)
	}
	datasetName := filepath.Base(cfg.DatasetPath)

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	type providerResult struct {
		provider BenchmarkProvider
		passed   int
		total    int
		latency  time.Duration
		cost     float64
		tokens   int
		err      error
	}

	resultChan := make(chan providerResult, len(providers))
	var wg sync.WaitGroup

	runOne := func(p BenchmarkProvider) providerResult {
		res := providerResult{provider: p}
		for _, tc := range ds.TestCases {
			caseCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			br, err := p.Run(caseCtx, tc.Prompt)
			if err != nil {
				res.total++
				continue
			}
			res.total++
			if br.Passed {
				res.passed++
			}
			res.latency += br.Latency
			res.cost += br.CostUSD
			res.tokens += br.Tokens
		}
		return res
	}

	for _, p := range providers {
		wg.Add(1)
		go func(prov BenchmarkProvider) {
			defer wg.Done()
			resultChan <- runOne(prov)
		}(p)
	}
	wg.Wait()
	close(resultChan)

	outcome := &BenchmarkOutcome{
		Category:  category,
		Dataset:   datasetName,
		Providers: len(providers),
		Cases:     len(ds.TestCases),
	}

	for pr := range resultChan {
		total := pr.total
		if total == 0 {
			total = 1
		}
		passRate := float64(pr.passed) / float64(total)
		avgLat := time.Duration(int64(pr.latency) / int64(total))
		avgCost := pr.cost / float64(total)
		avgTokens := pr.tokens / total

		outcome.Results = append(outcome.Results, PerProviderResult{
			Model:      pr.provider.Name(),
			PassRate:   passRate,
			AvgLatency: avgLat,
			AvgCost:    avgCost,
			AvgTokens:  avgTokens,
			Passed:     pr.passed,
			Total:      pr.total,
		})

		// Record into store
		_ = store.Upsert(ctx, PerfRecord{
			Model:        pr.provider.Name(),
			Category:     category,
			Dataset:      datasetName,
			PassRate:     passRate,
			AvgLatencyMs: avgLat.Milliseconds(),
			AvgCostUSD:   avgCost,
			AvgTokens:    avgTokens,
			SampleCount:  1,
			RecordedAt:   time.Now().UTC().Format(time.RFC3339),
		})
	}

	return outcome, nil
}
