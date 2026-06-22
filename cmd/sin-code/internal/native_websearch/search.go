// SPDX-License-Identifier: MIT
// Purpose: native Go websearch core for issue #381. Hosts the public
// Search() entry point that callers (the MCP bridge, the chat tools
// layer, the orchestrator's research subroutines) all consume. Backed
// by the public DuckDuckGo HTML endpoint so the function is usable
// with zero API key config; optional Bing / SerpAPI fallback is left
// to the future v3.24.0 multi-provider engine (out of scope here).
//
// Constraints honoured:
//   - M2 (single static binary): net/http + net/url only.
//   - M5 (module path): exports live under
//     github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/native_websearch.
//   - M7 (race-free): Cache and RateLimiter both own their own locks;
//     Client is safe for concurrent use and never mutates after build.
//   - robots.txt: parsed and cached the first time we touch a host; a
//     Disallow on the search path aborts the call cleanly instead of
//     hammering an endpoint we are not allowed to use.
package native_websearch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Result is one search-result row. The schema is intentionally
// minimal per the issue contract; richer Score/Rank fields belong in
// the multi-provider aggregator, not in the native fallback path.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Client is the public surface of the native websearch package. It is
// safe for concurrent use once built (mandate M7). Zero value is
// unusable; callers construct via NewClient.
type Client struct {
	httpClient  *http.Client
	userAgent   string
	endpoint    string
	maxResults  int
	cache       *Cache
	limiter     *RateLimiter
	robotsCache sync.Map
	robotsTTL   time.Duration
}

// NewClient returns a Client with sane defaults wired up: 15s HTTP
// timeout, the canonical DuckDuckGo HTML endpoint, an LRU cache large
// enough for a real session's worth of distinct queries, and the
// standard 5/1 burst ceiling for the DuckDuckGo HTML rate limiter.
// Tests inject a smaller cache / an unlimited limiter so the suite
// runs in microseconds.
func NewClient() *Client {
	return NewClientWithOptions(ClientOptions{})
}

// ClientOptions tweaks a Client at construction time. The zero value
// is the production default; tests pass a smaller cache / unlimited
// limiter to keep httptest.Server sandboxes fast.
type ClientOptions struct {
	HTTPClient *http.Client
	UserAgent  string
	Endpoint   string
	MaxResults int
	CacheSize  int
	CacheTTL   time.Duration
	Burst      int
	PerSecond  float64
	NoLimit    bool
}

// NewClientWithOptions returns a Client configured per opts. Zero opts
// collapses to NewClient(); negatives collapse to the production
// defaults composed in NewClient.
func NewClientWithOptions(opts ClientOptions) *Client {
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = "sin-code/1.0 (+native websearch)"
	}
	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = "https://html.duckduckgo.com/html"
	}
	max := opts.MaxResults
	if max <= 0 {
		max = 10
	}
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 15 * time.Minute
	}
	cacheSize := opts.CacheSize
	if cacheSize <= 0 {
		cacheSize = 64
	}
	burst := opts.Burst
	if burst <= 0 {
		burst = Cap
	}
	perSecond := opts.PerSecond
	if perSecond <= 0 {
		perSecond = PerSecond
	}
	var limiter *RateLimiter
	if opts.NoLimit {
		limiter = nil
	} else {
		limiter = NewRateLimiter(burst, perSecond)
	}
	return &Client{
		httpClient: hc,
		userAgent:  ua,
		endpoint:   endpoint,
		maxResults: max,
		cache:      NewCache(cacheSize, cacheTTL),
		limiter:    limiter,
		robotsTTL:  1 * time.Hour,
	}
}

// Cache exposes the underlying Cache so callers can read stats or
// drain entries for eviction tests; never mutate the cache pointer.
func (c *Client) Cache() *Cache { return c.cache }

// Limiter exposes the underlying RateLimiter so callers can read
// tokens-from-prod stats; nil when the Client was built with NoLimit.
func (c *Client) Limiter() *RateLimiter { return c.limiter }

// Search runs a query and returns up to maxResults rows. The contract:
//   - empty query → ErrEmptyQuery
//   - duplicate query within the cache TTL → served from cache, no network call
//   - bucket empty + ctx fires before refill → ctx.Err()
//   - robots.txt Disallow covers the search path → ErrDisallowed
//   - network or parse failure → wrapped error
//
// maxResults <= 0 collapses to the Client's configured default; this
// keeps the public signature stable while letting operator profiles
// shoulder the choice of "10 vs 50".
func (c *Client) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, ErrEmptyQuery
	}
	limit := maxResults
	if limit <= 0 {
		limit = c.maxResults
	}
	cacheKey := query
	if v, ok := c.cache.Get(cacheKey); ok {
		if rows, ok := v.([]Result); ok {
			if len(rows) > limit {
				rows = rows[:limit]
			}
			return rows, nil
		}
	}

	hostURL, err := url.Parse(c.endpoint)
	if err != nil {
		return nil, fmt.Errorf("native_websearch: bad endpoint: %w", err)
	}
	if err := c.checkRobots(ctx, hostURL.Scheme, hostURL.Host, hostURL.Path); err != nil {
		return nil, err
	}

	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	rows, err := c.fetchAndParse(ctx, query)
	if err != nil {
		return nil, err
	}
	c.cache.Put(cacheKey, rows)
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

// ErrEmptyQuery is returned when Search is called with an empty input.
// Distinguishing this from a network failure makes caller's UX easier
// to debug ("Did I forget to populate the prompt?" vs "Did the network
// break?"). It is exported so test-friendly callers can map it to a
// 400 instead of a 500.
var ErrEmptyQuery = errors.New("native_websearch: empty query")

// ErrDisallowed is returned when the target host's robots.txt forbids
// the search path the Client is configured for. Surfaced as a typed
// error so the MCP bridge can downgrade it to a permission-deny
// instead of a transient failure retry.
var ErrDisallowed = errors.New("native_websearch: disallowed by robots.txt")

// fetchAndParse hits the configured endpoint and decodes its HTML.
// It is split out from Search() so the cache/stats path can be tested
// without touching the network.
func (c *Client) fetchAndParse(ctx context.Context, query string) ([]Result, error) {
	q := url.Values{}
	q.Set("q", query)
	endpoint := c.endpoint
	if strings.HasSuffix(endpoint, "/") {
		endpoint = endpoint + "?" + q.Encode()
	} else {
		endpoint = endpoint + "/?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(q.Encode()))
	if err != nil {
		return nil, fmt.Errorf("native_websearch: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("native_websearch: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("native_websearch: rate-limited (status 429)")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("native_websearch: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("native_websearch: read body: %w", err)
	}
	return parseDuckDuckGoHTML(string(body)), nil
}

// parseDuckDuckGoHTML extracts result rows out of the DuckDuckGo HTML
// response. Pure stdlib string scanning so we avoid pulling
// golang.org/x/net/html into the binary (mandate M2). The scanner
// trusts the canonical result__a / result__snippet tag pair, with a
// small allowance for nested bold tags inside the title block.
func parseDuckDuckGoHTML(htmlBody string) []Result {
	out := []Result{}
	i := 0
	for {
		j := strings.Index(htmlBody[i:], `<a rel="nofollow" class="result__a" href="`)
		if j < 0 {
			break
		}
		abs := i + j + len(`<a rel="nofollow" class="result__a" href="`)
		closeIdx := strings.Index(htmlBody[abs:], `"`)
		if closeIdx < 0 {
			break
		}
		rawURL := htmlBody[abs : abs+closeIdx]
		realURL := unduckURL(rawURL)
		titleStart := abs + closeIdx + 2
		titleEnd := strings.Index(htmlBody[titleStart:], `</a>`)
		if titleEnd < 0 {
			break
		}
		title := strings.TrimSpace(htmlBody[titleStart : titleStart+titleEnd])
		i = titleStart + titleEnd + len(`</a>`)
		if strings.HasPrefix(title, "<") {
			if g := strings.Index(title, `>`); g >= 0 {
				title = title[g+1:]
			}
		}
		snippet := ""
		sIdx := strings.Index(htmlBody[i:], `<a class="result__snippet"`)
		if sIdx >= 0 {
			snipScan := htmlBody[i+sIdx:]
			sOpen := strings.Index(snipScan, `>`)
			sClose := strings.Index(snipScan, `</a>`)
			if sOpen >= 0 && sClose >= 0 && sClose > sOpen {
				snippet = strings.TrimSpace(snipScan[sOpen+1 : sClose])
				i = i + sIdx + sClose + len(`</a>`)
			}
		}
		out = append(out, Result{Title: title, URL: realURL, Snippet: snippet})
	}
	return out
}

// unduckURL peels the //duckduckgo.com/l/?uddg=<encoded> wrapper
// DuckDuckGo uses in its HTML results. Unwrapped URLs flow through
// unchanged so test fixtures can skip the rewrite path.
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

// robotsState tracks a single host's robots.txt freshness. The robots
// cache and the body cache share the LRU eviction discipline only
// loosely — a frozen snapshot is fine for robots.txt since the file
// changes on a human timescale, not a query timescale.
type robotsState struct {
	loadedAt time.Time
	patterns []string
}

// checkRobots loads (once per TTL) the host's robots.txt and returns
// ErrDisallowed if any Disallow rule covers the search path. The path
// argument is the URL path the Client will actually hit. Scheme is
// inherited from c.endpoint so the test httptest.Server (http://) and
// the production DuckDuckGo endpoint (https://) both resolve. We do
// not chase wildcard includes because DuckDuckGo itself keeps
// robots.txt flat. Allow rules are ignored — only Disallow can deny us.
func (c *Client) checkRobots(ctx context.Context, scheme, host, path string) error {
	if host == "" {
		return nil
	}
	if scheme == "" {
		scheme = "https"
	}
	now := time.Now()
	cacheKey := scheme + "://" + host
	if v, ok := c.robotsCache.Load(cacheKey); ok {
		state := v.(*robotsState)
		if now.Sub(state.loadedAt) < c.robotsTTL {
			return c.evalRobots(state, path)
		}
	}
	robotsURL := scheme + "://" + host + "/robots.txt"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	state := &robotsState{loadedAt: now, patterns: parseRobots(string(body))}
	c.robotsCache.Store(cacheKey, state)
	return c.evalRobots(state, path)
}

func (c *Client) evalRobots(state *robotsState, path string) error {
	candidate := path
	if candidate == "" {
		candidate = "/"
	}
	for _, p := range state.patterns {
		if p == "" {
			continue
		}
		if strings.HasPrefix(candidate, p) {
			return ErrDisallowed
		}
	}
	return nil
}

// parseRobots shreds a robots.txt body into the Disallow patterns we
// care about. User-agent specificity is flattened: if any agent lists
// the path, we honour the deny. This is conservative (we may skip too
// much) but never the other direction.
func parseRobots(body string) []string {
	out := []string{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "disallow:") {
			rest := strings.TrimSpace(line[len("disallow:"):])
			out = append(out, rest)
		}
	}
	return out
}
