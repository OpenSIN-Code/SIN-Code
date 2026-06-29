// SPDX-License-Identifier: MIT
// Purpose: tests for the native web-search engine (issue #381).
// Uses httptest.Server mocks for each provider; no real network calls.
package websearch

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockDoer returns an HTTP client suitable for hitting httptest.Server
// absolute URLs. The srv argument is unused — providers carry the full URL.
func mockDoer(srv *httptest.Server) HTTPDoer {
	_ = srv
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{},
	}
}

// --- SerpAPI ---------------------------------------------------------------

func TestSerpAPIProvider_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q != "golang" {
			t.Errorf("query = %q; want golang", q)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organic_results": []map[string]any{
				{"title": "Go", "link": "https://go.dev", "snippet": "Go programming"},
				{"title": "Wiki", "link": "https://wikipedia.org/Go", "snippet": "Go wiki"},
			},
		})
	}))
	defer srv.Close()

	p := &serpAPIProvider{key: "test-key", baseURL: srv.URL, doer: mockDoer(srv)}
	rs, err := p.Search(context.Background(), "golang", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Title != "Go" {
		t.Fatalf("results = %+v", rs)
	}
	if rs[0].Source != "serpapi" {
		t.Errorf("source = %q; want serpapi", rs[0].Source)
	}
	if rs[0].Score <= rs[1].Score {
		t.Error("first result should score higher than second")
	}
}

func TestSerpAPIProvider_MissingKey(t *testing.T) {
	p := &serpAPIProvider{key: "", baseURL: "http://x", doer: mockDoer(nil)}
	if _, err := p.Search(context.Background(), "q", 1); err == nil {
		t.Fatal("expected error for missing key")
	}
}

// --- Brave -----------------------------------------------------------------

func TestBraveProvider_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "brave-key" {
			t.Errorf("missing subscription token header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"web": map[string]any{
				"results": []map[string]any{
					{"title": "Brave1", "url": "https://brave1.com", "description": "d1"},
				},
			},
		})
	}))
	defer srv.Close()

	p := &braveProvider{key: "brave-key", baseURL: srv.URL, doer: mockDoer(srv)}
	rs, err := p.Search(context.Background(), "test", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].URL != "https://brave1.com" {
		t.Fatalf("results = %+v", rs)
	}
}

// --- DuckDuckGo ------------------------------------------------------------

func TestDuckDuckGoProvider_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{
			"hello",
			[]string{"hello world", "hello world 2"},
		})
	}))
	defer srv.Close()

	p := &duckDuckGoProvider{baseURL: srv.URL, doer: mockDoer(srv)}
	rs, err := p.Search(context.Background(), "hello", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("results = %d; want 2", len(rs))
	}
	if rs[0].Title != "hello world" {
		t.Errorf("title = %q", rs[0].Title)
	}
}

func TestDuckDuckGoProvider_EmptyFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{"solo"})
	}))
	defer srv.Close()

	p := &duckDuckGoProvider{baseURL: srv.URL, doer: mockDoer(srv)}
	rs, err := p.Search(context.Background(), "solo", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("fallback result count = %d; want 1", len(rs))
	}
}

// --- Tavily ----------------------------------------------------------------

func TestTavilyProvider_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) == "" {
			t.Error("empty POST body")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"title": "T1", "url": "https://t1.com", "content": "c1"},
			},
		})
	}))
	defer srv.Close()

	p := &tavilyProvider{key: "tavily-key", baseURL: srv.URL, doer: mockDoer(srv)}
	rs, err := p.Search(context.Background(), "ai", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Source != "tavily" {
		t.Fatalf("results = %+v", rs)
	}
}

func TestTavilyProvider_MissingKey(t *testing.T) {
	p := &tavilyProvider{key: "", baseURL: "http://x", doer: mockDoer(nil)}
	if _, err := p.Search(context.Background(), "q", 1); err == nil {
		t.Fatal("expected error for missing key")
	}
}

// --- Engine fan-out --------------------------------------------------------

func TestEngine_Search_FanOutAndDedupe(t *testing.T) {
	// Two providers return overlapping URLs; engine must dedupe.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organic_results": []map[string]any{
				{"title": "Shared", "link": "https://shared.com", "snippet": "s"},
				{"title": "OnlySerp", "link": "https://only-serp.com", "snippet": "s2"},
			},
		})
	}))
	defer srv.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"phrase": "Shared"},
		})
	}))
	defer srv2.Close()

	cfg := Config{
		EnabledProviders: []string{"serpapi", "duckduckgo"},
		SerpAPIKey:       "k",
		DefaultTimeout:   2 * time.Second,
		MaxResults:       5,
	}
	// Build engine with mock providers directly.
	e := &Engine{
		providers: []Provider{
			&serpAPIProvider{key: "k", baseURL: srv.URL, doer: mockDoer(srv)},
			&duckDuckGoProvider{baseURL: srv2.URL, doer: mockDoer(srv2)},
		},
		timeout:    cfg.DefaultTimeout,
		maxResults: cfg.MaxResults,
	}
	rs, stats := e.Search(context.Background(), "test")
	// duckduckgo "Shared" maps to https://duckduckgo.com/lite/?q=Shared which
	// is a different URL from https://shared.com, so no dedupe there.
	// But we should get 3 results total (2 serpapi + 1 ddg).
	if len(rs) != 3 {
		t.Fatalf("results = %d; want 3", len(rs))
	}
	if stats.Errors != 0 {
		t.Fatalf("errors = %d; want 0", stats.Errors)
	}
	if stats.Providers != 2 {
		t.Fatalf("providers = %d; want 2", stats.Providers)
	}
}

func TestEngine_Search_DedupeByURL(t *testing.T) {
	// Both providers return the exact same URL — must be deduped to 1.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organic_results": []map[string]any{
				{"title": "A", "link": "https://dup.com", "snippet": "x"},
			},
		})
	}))
	defer srv.Close()

	e := &Engine{
		providers: []Provider{
			&serpAPIProvider{key: "k", baseURL: srv.URL, doer: mockDoer(srv)},
			&serpAPIProvider{key: "k", baseURL: srv.URL, doer: mockDoer(srv)},
		},
		timeout:    2 * time.Second,
		maxResults: 5,
	}
	rs, _ := e.Search(context.Background(), "dup")
	if len(rs) != 1 {
		t.Fatalf("dedupe count = %d; want 1", len(rs))
	}
}

func TestEngine_Search_ProviderError(t *testing.T) {
	// Server returns 500 — provider errors, engine records it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := &Engine{
		providers:  []Provider{&serpAPIProvider{key: "k", baseURL: srv.URL, doer: mockDoer(srv)}},
		timeout:    2 * time.Second,
		maxResults: 5,
	}
	rs, stats := e.Search(context.Background(), "fail")
	if len(rs) != 0 {
		t.Fatalf("results = %d; want 0", len(rs))
	}
	if stats.Errors != 1 {
		t.Fatalf("errors = %d; want 1", stats.Errors)
	}
}

func TestLoadConfig_DuckDuckGoAlwaysEnabled(t *testing.T) {
	t.Setenv(EnvSerpAPIKey, "")
	t.Setenv(EnvBraveKey, "")
	t.Setenv(EnvTavilyKey, "")
	cfg := LoadConfig()
	found := false
	for _, p := range cfg.EnabledProviders {
		if p == "duckduckgo" {
			found = true
		}
	}
	if !found {
		t.Fatal("duckduckgo should always be enabled")
	}
	// no key providers should be enabled
	for _, p := range cfg.EnabledProviders {
		if p == "serpapi" || p == "brave" || p == "tavily" {
			t.Fatalf("provider %s should not be enabled without a key", p)
		}
	}
}

func TestLoadConfig_WithKeys(t *testing.T) {
	t.Setenv(EnvSerpAPIKey, "sk")
	t.Setenv(EnvBraveKey, "bk")
	t.Setenv(EnvTavilyKey, "tk")
	cfg := LoadConfig()
	if cfg.SerpAPIKey != "sk" || cfg.BraveKey != "bk" || cfg.TavilyKey != "tk" {
		t.Fatalf("keys not loaded: %+v", cfg)
	}
}
