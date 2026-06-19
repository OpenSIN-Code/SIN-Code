// SPDX-License-Identifier: MIT
// Purpose: core engine for native multi-provider web search (issue #381).
// Fans out queries to all enabled providers in parallel, deduplicates
// results by URL, and ranks them by a composite score. Race-safe (M7):
// the result merge uses a mutex-protected collector.
package websearch

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Result is a single search hit, normalised across all providers.
type Result struct {
	Title   string
	URL     string
	Snippet string
	Source  string // provider name that returned this result
	Score   float64
}

// Provider is the interface every search backend implements.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string, maxResults int) ([]Result, error)
}

// Stats reports per-provider success/failure counts for a Search call.
type Stats struct {
	Providers  int
	Results    int
	Errors     int
	DurationMS int64
}

// Engine orchestrates fan-out search across multiple providers.
type Engine struct {
	providers  []Provider
	timeout    time.Duration
	maxResults int
}

// NewEngine builds an Engine from a Config and an HTTPDoer. Only providers
// whose names appear in cfg.EnabledProviders are wired in.
func NewEngine(cfg Config, doer HTTPDoer) *Engine {
	if doer == nil {
		doer = NewStdlibDoer(cfg.DefaultTimeout)
	}
	e := &Engine{
		timeout:    cfg.DefaultTimeout,
		maxResults: cfg.MaxResults,
	}
	registry := map[string]func(Config, HTTPDoer) Provider{
		"serpapi":    newSerpAPIProvider,
		"brave":      newBraveProvider,
		"duckduckgo": newDuckDuckGoProvider,
		"tavily":     newTavilyProvider,
	}
	for _, name := range cfg.EnabledProviders {
		if ctor, ok := registry[name]; ok {
			e.providers = append(e.providers, ctor(cfg, doer))
		}
	}
	return e
}

// Search fans out to all enabled providers in parallel, merges, dedupes
// by URL, ranks by score, and returns the top maxResults hits plus Stats.
func (e *Engine) Search(ctx context.Context, query string) ([]Result, Stats) {
	start := time.Now()
	max := e.maxResults
	if max <= 0 {
		max = 10
	}

	type providerResults struct {
		name    string
		results []Result
		err     error
	}

	var wg sync.WaitGroup
	ch := make(chan providerResults, len(e.providers))
	for _, p := range e.providers {
		wg.Add(1)
		go func(pv Provider) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, e.timeout)
			defer cancel()
			rs, err := pv.Search(pctx, query, max)
			ch <- providerResults{name: pv.Name(), results: rs, err: err}
		}(p)
	}
	wg.Wait()
	close(ch)

	var all []Result
	errors := 0
	for pr := range ch {
		if pr.err != nil {
			errors++
			continue
		}
		all = append(all, pr.results...)
	}

	merged := dedupeAndRank(all, max)
	stats := Stats{
		Providers:  len(e.providers),
		Results:    len(merged),
		Errors:     errors,
		DurationMS: time.Since(start).Milliseconds(),
	}
	return merged, stats
}

// dedupeAndRank removes duplicate URLs (keeping the highest-scored
// instance), then sorts by score descending and truncates to max.
func dedupeAndRank(in []Result, max int) []Result {
	seen := make(map[string]int, len(in))
	var out []Result
	for _, r := range in {
		key := strings.ToLower(strings.TrimSpace(r.URL))
		if key == "" {
			continue
		}
		if idx, ok := seen[key]; ok {
			if r.Score > out[idx].Score {
				out[idx] = r
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	if len(out) > max {
		out = out[:max]
	}
	return out
}
