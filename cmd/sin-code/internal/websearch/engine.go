// SPDX-License-Identifier: MIT
// Purpose: Go-native multi-provider web search aggregator (issue #381).
// Mirrors the public surface of the external `websearch` MCP skill
// (`sin-websearch` / `websearch__search`) so callers can swap
// implementations without rippling through the agent loop. Race-clean,
// dependency-free, stdlib HTTP only. Default activation is zero-cost
// (every provider is a no-op stub until a key env var lights it up).
package websearch

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Result is one search-result row. The schema is byte-compatible with the
// JSON the legacy `sin_web_search` chat tool emits so callers can treat
// native and MCP outputs interchangeably.
type Result struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// Provider is the pluggable search-source contract implemented by every
// backend below. Implementations must be safe for concurrent use because
// Engine fans a query out across all providers in parallel.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string) ([]Result, error)
}

// Config configures an Engine. Missing API keys leave the corresponding
// provider registered but inactive (Search returns no rows for it).
type Config struct {
	SerpAPIKey string
	BraveKey   string
	BingKey    string
	TavilyKey  string
	UserAgent  string
	Timeout    time.Duration
	HTTPClient HTTPDoer
}

// HTTPDoer abstracts net/http.Client so tests can inject an httptest.Server
// without spinners. The zero value uses stdlib http.DefaultClient.
type HTTPDoer interface {
	Do(req *httpRequest) (*httpResponse, error)
}

// Stats is the engine footprint report. ActiveNames is a stable, alphabetically
// sorted snapshot of the registered providers so callers can pin byte-
// stable telemetry.
type Stats struct {
	Providers   int      `json:"providers"`
	ActiveNames []string `json:"active_names"`
}

// Engine fans a query out across multiple providers, dedupes by URL,
// and ranks by Score (descending). Zero value is unusable; always
// construct with NewEngine.
type Engine struct {
	mu        sync.RWMutex
	cfg       Config
	providers []Provider
}

// NewEngine returns an Engine pre-populated with the five canonical
// providers (SerpAPI, Brave, Bing, DuckDuckGo, Tavily). Providers that
// need an API key but find it missing deactivate themselves and silently
// return no rows — they never fail a Search call.
func NewEngine(cfg Config) *Engine {
	e := &Engine{cfg: cfg}
	e.AddProvider(NewSerpAPIProvider(cfg))
	e.AddProvider(NewBraveProvider(cfg))
	e.AddProvider(NewBingProvider(cfg))
	e.AddProvider(NewDuckDuckGoProvider(cfg))
	e.AddProvider(NewTavilyProvider(cfg))
	return e
}

// AddProvider registers a provider. Nil and duplicates by Name are
// silently ignored so the engine remains deterministic.
func (e *Engine) AddProvider(p Provider) {
	if p == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, existing := range e.providers {
		if existing.Name() == p.Name() {
			return
		}
	}
	e.providers = append(e.providers, p)
}

// Providers returns a copy of the registered provider list so callers
// cannot mutate the engine's internal slice.
func (e *Engine) Providers() []Provider {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Provider, len(e.providers))
	copy(out, e.providers)
	return out
}

// Search fans a query out across all registered providers, dedupes
// results by URL (first write wins, highest score wins for ties),
// and returns the ranked list. An empty query or zero registered
// providers returns a descriptive error.
func (e *Engine) Search(ctx context.Context, query string) ([]Result, error) {
	if query == "" {
		return nil, errors.New("websearch: empty query")
	}
	e.mu.RLock()
	providers := make([]Provider, len(e.providers))
	copy(providers, e.providers)
	e.mu.RUnlock()

	if len(providers) == 0 {
		return nil, errors.New("websearch: no providers registered")
	}

	timeout := e.cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	type namedResult struct {
		name string
		rows []Result
		err  error
	}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
		rs = make([]namedResult, 0, len(providers))
	)
	for _, p := range providers {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			pCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			rows, err := p.Search(pCtx, query)
			mu.Lock()
			defer mu.Unlock()
			rs = append(rs, namedResult{name: p.Name(), rows: rows, err: err})
		}(p)
	}
	wg.Wait()

	// Sort results so registration order is preserved during dedup;
	// providers can finish in any order, so we keep the original
	// index alongside the result. First writer wins for any given URL.
	type indexed struct {
		idx int
		rows []Result
		err  error
	}
	indexedResults := make([]indexed, len(providers))
	providerNames := make([]string, len(providers))
	for i, p := range providers {
		indexedResults[i] = indexed{idx: i}
		providerNames[i] = p.Name()
	}
	nameToIdx := make(map[string]int, len(providerNames))
	for i, n := range providerNames {
		nameToIdx[n] = i
	}
	// Map goroutine outputs back to their registration index.
	placeholder := map[int]bool{}
	for _, nr := range rs {
		if idx, ok := nameToIdx[nr.name]; ok && !placeholder[idx] {
			indexedResults[idx] = indexed{idx: idx, rows: nr.rows, err: nr.err}
			placeholder[idx] = true
		}
	}

	seen := make(map[string]bool, 32)
	merged := make([]Result, 0, 32)
	for _, ir := range indexedResults {
		if ir.err != nil {
			continue
		}
		for _, r := range ir.rows {
			if r.URL == "" {
				continue
			}
			if seen[r.URL] {
				continue
			}
			seen[r.URL] = true
			merged = append(merged, r)
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return merged[i].URL < merged[j].URL
	})
	if len(merged) == 0 {
		return nil, errors.New("websearch: no results from any active provider")
	}
	return merged, nil
}

// Stats reports the current provider footprint with a stable,
// alphabetically sorted active-names list.
func (e *Engine) Stats() Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	names := make([]string, 0, len(e.providers))
	for _, p := range e.providers {
		names = append(names, p.Name())
	}
	sort.Strings(names)
	return Stats{
		Providers:   len(e.providers),
		ActiveNames: names,
	}
}
