// SPDX-License-Identifier: MIT
// Purpose: five canonical web-search providers (SerpAPI, Brave, Bing,
// DuckDuckGo, Tavily) wired into the engine. Every provider is a stub:
// gated by API key, no-op when the key is missing, with a real HTTP
// adapter behind it so test suites can run an httptest.Server against
// the same code path callers use against the real APIs.
package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- helpers ---------------------------------------------------------------

func newHeader() http.Header {
	return http.Header{}
}

func pickDoer(cfg Config) HTTPDoer {
	if cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	return newStdlibDoer(nil)
}

// --- SerpAPI ---------------------------------------------------------------

// SerpAPIProvider is the SerpAPI search backend
// (https://serpapi.com/search-api). JSON in, JSON out. Requires
// WEBSEARCH_SERPAPI_KEY env var to activate; without it the provider
// is a no-op so an Engine call with no key still works.
type SerpAPIProvider struct {
	key  string
	doer HTTPDoer
}

// NewSerpAPIProvider returns a SerpAPI provider wired into cfg. It is
// safe to register against any Engine; an empty key activates the
// no-op path.
func NewSerpAPIProvider(cfg Config) *SerpAPIProvider {
	return &SerpAPIProvider{key: cfg.SerpAPIKey, doer: pickDoer(cfg)}
}

// Name implements Provider.
func (p *SerpAPIProvider) Name() string { return "serpapi" }

// Search implements Provider. Empty-key activation is a silent no-op.
func (p *SerpAPIProvider) Search(_ context.Context, query string) ([]Result, error) {
	if p.key == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("api_key", p.key)
	q.Set("engine", "google")
	endpoint := "https://serpapi.com/search.json?" + q.Encode()
	req := &httpRequest{method: "GET", url: endpoint}
	hdr := newHeader()
	hdr.Set("Accept", "application/json")
	req.header = hdr
	resp, err := p.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("serpapi: %w", err)
	}
	if resp.status >= 400 {
		return nil, fmt.Errorf("serpapi: status %d", resp.status)
	}
	var body struct {
		OrganicResults []struct {
			Title    string `json:"title"`
			Link     string `json:"link"`
			Snippet  string `json:"snippet"`
			Position int    `json:"position"`
		} `json:"organic_results"`
	}
	if err := json.Unmarshal(resp.body, &body); err != nil {
		return nil, fmt.Errorf("serpapi: decode: %w", err)
	}
	out := make([]Result, 0, len(body.OrganicResults))
	for _, r := range body.OrganicResults {
		score := 1.0 / float64(1+r.Position)
		out = append(out, Result{Title: r.Title, URL: r.Link, Snippet: r.Snippet, Score: score})
	}
	return out, nil
}

// --- Brave -----------------------------------------------------------------

// BraveProvider is the Brave Search API backend. JSON over HTTPS. Requires
// WEBSEARCH_BRAVE_KEY; without it the provider is a silent no-op.
type BraveProvider struct {
	key  string
	doer HTTPDoer
}

func NewBraveProvider(cfg Config) *BraveProvider {
	return &BraveProvider{key: cfg.BraveKey, doer: pickDoer(cfg)}
}

func (p *BraveProvider) Name() string { return "brave" }

func (p *BraveProvider) Search(_ context.Context, query string) ([]Result, error) {
	if p.key == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("count", "10")
	endpoint := "https://api.search.brave.com/res/v1/web/search?" + q.Encode()
	req := &httpRequest{method: "GET", url: endpoint}
	hdr := newHeader()
	hdr.Set("Accept", "application/json")
	hdr.Set("X-Subscription-Token", p.key)
	req.header = hdr
	resp, err := p.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave: %w", err)
	}
	if resp.status >= 400 {
		return nil, fmt.Errorf("brave: status %d", resp.status)
	}
	var body struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(resp.body, &body); err != nil {
		return nil, fmt.Errorf("brave: decode: %w", err)
	}
	out := make([]Result, 0, len(body.Web.Results))
	for i, r := range body.Web.Results {
		score := 1.0 / float64(1+i)
		out = append(out, Result{Title: r.Title, URL: r.URL, Snippet: r.Description, Score: score})
	}
	return out, nil
}

// --- Bing ------------------------------------------------------------------

// BingProvider is the Bing Web Search API v7. Requires an Ocp-Apim-Subscription-
// Key header; the env var is WEBSEARCH_BING_KEY. Without a key it returns [].
type BingProvider struct {
	key  string
	doer HTTPDoer
}

func NewBingProvider(cfg Config) *BingProvider {
	return &BingProvider{key: cfg.BingKey, doer: pickDoer(cfg)}
}

func (p *BingProvider) Name() string { return "bing" }

func (p *BingProvider) Search(_ context.Context, query string) ([]Result, error) {
	if p.key == "" {
		return nil, nil
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("count", "10")
	endpoint := "https://api.bing.microsoft.com/v7.0/search?" + q.Encode()
	req := &httpRequest{method: "GET", url: endpoint}
	hdr := newHeader()
	hdr.Set("Accept", "application/json")
	hdr.Set("Ocp-Apim-Subscription-Key", p.key)
	req.header = hdr
	resp, err := p.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bing: %w", err)
	}
	if resp.status >= 400 {
		return nil, fmt.Errorf("bing: status %d", resp.status)
	}
	var body struct {
		WebPages struct {
			Value []struct {
				Name    string `json:"name"`
				URL     string `json:"url"`
				Snippet string `json:"snippet"`
			} `json:"value"`
		} `json:"webPages"`
	}
	if err := json.Unmarshal(resp.body, &body); err != nil {
		return nil, fmt.Errorf("bing: decode: %w", err)
	}
	out := make([]Result, 0, len(body.WebPages.Value))
	for i, r := range body.WebPages.Value {
		score := 1.0 / float64(1+i)
		out = append(out, Result{Title: r.Name, URL: r.URL, Snippet: r.Snippet, Score: score})
	}
	return out, nil
}

// --- DuckDuckGo ------------------------------------------------------------

// DuckDuckGoProvider is the keyless fallback. It scrapes the HTML
// endpoint at html.duckduckgo.com (the same path the legacy Python
// `duckduckgo_search` bridge used). No key required, low rate ceiling;
// always last in the priority list.
type DuckDuckGoProvider struct {
	doer HTTPDoer
}

func NewDuckDuckGoProvider(cfg Config) *DuckDuckGoProvider {
	return &DuckDuckGoProvider{doer: pickDoer(cfg)}
}

func (p *DuckDuckGoProvider) Name() string { return "duckduckgo" }

func (p *DuckDuckGoProvider) Search(_ context.Context, query string) ([]Result, error) {
	q := url.Values{}
	q.Set("q", query)
	endpoint := "https://html.duckduckgo.com/html/?" + q.Encode()
	req := &httpRequest{method: "POST", url: endpoint}
	hdr := newHeader()
	hdr.Set("Content-Type", "application/x-www-form-urlencoded")
	hdr.Set("Accept", "text/html")
	req.header = hdr
	req.body = []byte(q.Encode())
	resp, err := p.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo: %w", err)
	}
	if resp.status >= 400 {
		return nil, fmt.Errorf("duckduckgo: status %d", resp.status)
	}
	return parseDuckDuckGoHTML(string(resp.body)), nil
}

// parseDuckDuckGoHTML extracts the result links out of html.duckduckgo.com's
// HTML response. Trusts the canonical result__a / result__snippet tag pair.
func parseDuckDuckGoHTML(html string) []Result {
	out := []Result{}
	idx := 0
	i := 0
	for {
		j := strings.Index(html[i:], `<a rel="nofollow" class="result__a" href="`)
		if j < 0 {
			break
		}
		abs := i + j + len(`<a rel="nofollow" class="result__a" href="`)
		closeIdx := strings.Index(html[abs:], `"`)
		if closeIdx < 0 {
			break
		}
		rawURL := html[abs : abs+closeIdx]
		realURL := unduckURL(rawURL)
		// Skip past the closing quote AND the opening '>' of the <a> tag.
		titleStart := abs + closeIdx + 2
		titleEnd := strings.Index(html[titleStart:], `</a>`)
		if titleEnd < 0 {
			break
		}
		title := strings.TrimSpace(html[titleStart : titleStart+titleEnd])
		i = titleStart + titleEnd + len(`</a>`)
		// DuckDuckGo sometimes wraps the title in additional nested tags
		// like <b class="result__title">...</b>. Strip a leading < matches
		// until we hit actual text content.
		if strings.HasPrefix(title, "<") {
			// Find the first '>' inside the leading tag block, drop everything up to and including it.
			if g := strings.Index(title, `>`); g >= 0 {
				title = title[g+1:]
			}
		}
		snippet := ""
		sIdx := strings.Index(html[i:], `<a class="result__snippet"`)
		if sIdx >= 0 {
			snipScan := html[i+sIdx:]
			sOpen := strings.Index(snipScan, `>`)
			sClose := strings.Index(snipScan, `</a>`)
			if sOpen >= 0 && sClose >= 0 && sClose > sOpen {
				snippet = strings.TrimSpace(snipScan[sOpen+1 : sClose])
				i = i + sIdx + sClose + len(`</a>`)
			}
		}
		out = append(out, Result{
			Title:   title,
			URL:     realURL,
			Snippet: snippet,
			Score:   1.0 / float64(1+idx),
		})
		idx++
	}
	return out
}

// unduckURL peels the //duckduckgo.com/l/?uddg=<encoded> wrapper DuckDuckGo
// uses in its HTML results. If the URL is already a real one it's returned
// unchanged.
func unduckURL(raw string) string {
	if !strings.Contains(raw, "uddg=") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	real := u.Query().Get("uddg")
	if real == "" {
		return raw
	}
	return real
}

// --- Tavily ----------------------------------------------------------------

// TavilyProvider targets https://api.tavily.com/search. Requires WEBSEARCH_TAVILY_KEY.
type TavilyProvider struct {
	key  string
	doer HTTPDoer
}

func NewTavilyProvider(cfg Config) *TavilyProvider {
	return &TavilyProvider{key: cfg.TavilyKey, doer: pickDoer(cfg)}
}

func (p *TavilyProvider) Name() string { return "tavily" }

func (p *TavilyProvider) Search(_ context.Context, query string) ([]Result, error) {
	if p.key == "" {
		return nil, nil
	}
	bodyJSON, err := json.Marshal(map[string]any{
		"api_key":      p.key,
		"query":        query,
		"max_results":  10,
		"search_depth": "basic",
	})
	if err != nil {
		return nil, fmt.Errorf("tavily: marshal: %w", err)
	}
	req := &httpRequest{
		method: "POST",
		url:    "https://api.tavily.com/search",
		body:   bodyJSON,
	}
	hdr := newHeader()
	hdr.Set("Content-Type", "application/json")
	req.header = hdr
	resp, err := p.doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily: %w", err)
	}
	if resp.status >= 400 {
		return nil, fmt.Errorf("tavily: status %d", resp.status)
	}
	var body struct {
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp.body, &body); err != nil {
		return nil, fmt.Errorf("tavily: decode: %w", err)
	}
	out := make([]Result, 0, len(body.Results))
	for _, r := range body.Results {
		score := r.Score
		if score == 0 {
			score = 0.5
		}
		out = append(out, Result{Title: r.Title, URL: r.URL, Snippet: r.Content, Score: score})
	}
	return out, nil
}
