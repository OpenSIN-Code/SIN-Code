// SPDX-License-Identifier: MIT
// Purpose: concrete search-provider implementations (issue #381).
//   - SerpAPIProvider  — paid, gated by WEBSEARCH_SERPAPI_KEY
//   - BraveProvider    — paid, gated by WEBSEARCH_BRAVE_KEY
//   - DuckDuckGoProvider — keyless HTML scraping (instant answer / lite)
//   - TavilyProvider   — AI search, gated by WEBSEARCH_TAVILY_KEY
// Each provider normalises its backend response into []Result. Scoring:
// the first results rank higher (position-based score so the engine's
// cross-provider merge can interleave meaningfully).
package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// --- shared helpers --------------------------------------------------------

func scoreForPosition(pos, total int) float64 {
	if total <= 0 {
		return 0
	}
	if pos < 0 {
		pos = 0
	}
	return float64(total-pos) / float64(total)
}

func doJSON(ctx context.Context, doer HTTPDoer, method, rawURL string, headers map[string]string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := doer.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider HTTP %d: %s", resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

// --- SerpAPI ---------------------------------------------------------------

type serpAPIProvider struct {
	key     string
	baseURL string
	doer    HTTPDoer
}

const defaultSerpAPIBase = "https://serpapi.com/search.json"

func newSerpAPIProvider(cfg Config, doer HTTPDoer) Provider {
	return &serpAPIProvider{key: cfg.SerpAPIKey, baseURL: defaultSerpAPIBase, doer: doer}
}

func (s *serpAPIProvider) Name() string { return "serpapi" }

func (s *serpAPIProvider) Search(ctx context.Context, query string, max int) ([]Result, error) {
	if s.key == "" {
		return nil, fmt.Errorf("serpapi: missing API key")
	}
	u := s.baseURL + "?engine=google&q=" + url.QueryEscape(query) +
		"&api_key=" + url.QueryEscape(s.key) + "&num=" + fmt.Sprint(max)
	b, err := doJSON(ctx, s.doer, http.MethodGet, u, nil, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("serpapi: decode: %w", err)
	}
	total := len(raw.Organic)
	out := make([]Result, 0, total)
	for i, o := range raw.Organic {
		out = append(out, Result{
			Title: o.Title, URL: o.Link, Snippet: o.Snippet,
			Source: "serpapi", Score: scoreForPosition(i, total),
		})
	}
	return out, nil
}

// --- Brave -----------------------------------------------------------------

type braveProvider struct {
	key     string
	baseURL string
	doer    HTTPDoer
}

const defaultBraveBase = "https://api.search.brave.com/res/v1/web/search"

func newBraveProvider(cfg Config, doer HTTPDoer) Provider {
	return &braveProvider{key: cfg.BraveKey, baseURL: defaultBraveBase, doer: doer}
}

func (b *braveProvider) Name() string { return "brave" }

func (b *braveProvider) Search(ctx context.Context, query string, max int) ([]Result, error) {
	if b.key == "" {
		return nil, fmt.Errorf("brave: missing API key")
	}
	u := b.baseURL + "?q=" + url.QueryEscape(query) + "&count=" + fmt.Sprint(max)
	bb, err := doJSON(ctx, b.doer, http.MethodGet, u, map[string]string{
		"X-Subscription-Token": b.key,
		"Accept":               "application/json",
	}, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(bb, &raw); err != nil {
		return nil, fmt.Errorf("brave: decode: %w", err)
	}
	rs := raw.Web.Results
	out := make([]Result, 0, len(rs))
	for i, r := range rs {
		out = append(out, Result{
			Title: r.Title, URL: r.URL, Snippet: r.Description,
			Source: "brave", Score: scoreForPosition(i, len(rs)),
		})
	}
	return out, nil
}

// --- DuckDuckGo (keyless HTML) ---------------------------------------------

type duckDuckGoProvider struct {
	baseURL string
	doer    HTTPDoer
}

const defaultDuckDuckGoBase = "https://duckduckgo.com/ac/"

func newDuckDuckGoProvider(cfg Config, doer HTTPDoer) Provider {
	return &duckDuckGoProvider{baseURL: defaultDuckDuckGoBase, doer: doer}
}

func (d *duckDuckGoProvider) Name() string { return "duckduckgo" }

// DuckDuckGo's instant-answer JSON endpoint (duckduckgo.com/ac/?q=...) is
// keyless. It returns suggestion objects; we treat each suggestion as a
// result pointing to the DuckDuckGo lite page for that query.
func (d *duckDuckGoProvider) Search(ctx context.Context, query string, max int) ([]Result, error) {
	u := d.baseURL + "?q=" + url.QueryEscape(query) + "&type=list"
	b, err := doJSON(ctx, d.doer, http.MethodGet, u, map[string]string{
		"Accept": "application/json",
	}, nil)
	if err != nil {
		return nil, err
	}
	// The endpoint returns either a list of {phrase:"..."} objects or a
	// nested list form. We handle the common object form.
	var suggestions []struct {
		Phrase string `json:"phrase"`
	}
	if err := json.Unmarshal(b, &suggestions); err != nil {
		return nil, fmt.Errorf("duckduckgo: decode: %w", err)
	}
	if max > 0 && len(suggestions) > max {
		suggestions = suggestions[:max]
	}
	out := make([]Result, 0, len(suggestions))
	for i, s := range suggestions {
		link := "https://duckduckgo.com/lite/?q=" + url.QueryEscape(s.Phrase)
		out = append(out, Result{
			Title: s.Phrase, URL: link, Snippet: s.Phrase,
			Source: "duckduckgo", Score: scoreForPosition(i, len(suggestions)),
		})
	}
	if len(out) == 0 {
		// Fallback: at least one result pointing at the lite search page.
		link := "https://duckduckgo.com/lite/?q=" + url.QueryEscape(query)
		out = append(out, Result{
			Title: query, URL: link, Snippet: strings.TrimSpace(query),
			Source: "duckduckgo", Score: 0.5,
		})
	}
	return out, nil
}

// --- Tavily ----------------------------------------------------------------

type tavilyProvider struct {
	key     string
	baseURL string
	doer    HTTPDoer
}

const defaultTavilyBase = "https://api.tavily.com/search"

func newTavilyProvider(cfg Config, doer HTTPDoer) Provider {
	return &tavilyProvider{key: cfg.TavilyKey, baseURL: defaultTavilyBase, doer: doer}
}

func (t *tavilyProvider) Name() string { return "tavily" }

func (t *tavilyProvider) Search(ctx context.Context, query string, max int) ([]Result, error) {
	if t.key == "" {
		return nil, fmt.Errorf("tavily: missing API key")
	}
	payload := fmt.Sprintf(`{"api_key":%q,"query":%q,"max_results":%d}`, t.key, query, max)
	u := t.baseURL
	b, err := doJSON(ctx, t.doer, http.MethodPost, u, map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	var raw struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("tavily: decode: %w", err)
	}
	total := len(raw.Results)
	out := make([]Result, 0, total)
	for i, r := range raw.Results {
		out = append(out, Result{
			Title: r.Title, URL: r.URL, Snippet: r.Content,
			Source: "tavily", Score: scoreForPosition(i, total),
		})
	}
	return out, nil
}
