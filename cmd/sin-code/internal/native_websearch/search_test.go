// SPDX-License-Identifier: MIT
// Purpose: race-clean unit tests for the native websearch package
// (issue #381). All network surfaces are mocked with httptest.Server so
// the suite runs in milliseconds and never reaches the public internet.
// Run with `go test -race -count=1` to satisfy mandate M7.
package native_websearch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const ddgFixture = `<html><body>
<div class="result results_links results_links_deep web-result">
  <h2 class="result__title">
    <a rel="nofollow" class="result__a" href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fgolang.org%2F&kl=us-en">Go Programming Language</a>
  </h2>
  <a class="result__snippet" href="https://golang.org">The Go programming language is an open source project to make programmers more productive.</a>
</div>
<div class="result results_links results_links_deep web-result">
  <h2 class="result__title">
    <a rel="nofollow" class="result__a" href="https://duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev%2F&kl=us-en">go.dev</a>
  </h2>
  <a class="result__snippet" href="https://go.dev">Home of the Go programming language, tutorials, and downloads.</a>
</div>
<div class="result results_links results_links_deep web-result">
  <h2 class="result__title">
    <a rel="nofollow" class="result__a" href="https://example.com/3">Third result (already unwrapped)</a>
  </h2>
  <a class="result__snippet" href="https://example.com/3">No snippet available.</a>
</div>
</body></html>`

const bingFixtureHTML = `<html><body>
<div class="result results_links results_links_deep web-result">
  <h2 class="result__title">
    <a rel="nofollow" class="result__a" href="https://example.com/go">Why Go?</a>
  </h2>
  <a class="result__snippet" href="https://example.com/go">Because static binaries and concurrency.</a>
</div>
</body></html>`

// newMockServer returns an httptest.Server that serves the DuckDuckGo
// fixture at any path, and a configured robots.txt at the well-known
// /robots.txt location.
func newMockServer(t *testing.T, html string, robotsBody string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(robotsBody))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(html))
	})
	return httptest.NewServer(mux)
}

func permissiveRobots() string {
	return "User-agent: *\nDisallow:\n"
}

func TestSearchWithMockedHTML(t *testing.T) {
	srv := newMockServer(t, ddgFixture, permissiveRobots())
	defer srv.Close()
	cli := NewClientWithOptions(ClientOptions{
		Endpoint: srv.URL,
		NoLimit:  true,
		CacheTTL: time.Minute,
	})
	rows, err := cli.Search(context.Background(), "go programming language", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %#v", len(rows), rows)
	}
	if rows[0].Title != "Go Programming Language" {
		t.Errorf("row[0].Title = %q, want %q", rows[0].Title, "Go Programming Language")
	}
	if rows[0].URL != "https://golang.org/" {
		t.Errorf("row[0].URL = %q, want %q (unwrapped from duckduckgo.com/l/?uddg)", rows[0].URL, "https://golang.org/")
	}
	if rows[0].Snippet == "" {
		t.Errorf("row[0].Snippet empty; expected DuckDuckGo-shaped snippet text")
	}
	if rows[2].URL != "https://example.com/3" {
		t.Errorf("row[2].URL = %q, want unwrapped %q", rows[2].URL, "https://example.com/3")
	}
}

func TestSearchSnippetsAndLimit(t *testing.T) {
	srv := newMockServer(t, ddgFixture, permissiveRobots())
	defer srv.Close()
	cli := NewClientWithOptions(ClientOptions{
		Endpoint: srv.URL,
		NoLimit:  true,
	})
	rows, err := cli.Search(context.Background(), "go", 2)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("maxResults=2 but got %d rows", len(rows))
	}
	for i, r := range rows {
		if r.Title == "" || r.URL == "" {
			t.Errorf("row[%d] missing required field: %#v", i, r)
		}
	}
}

func TestSearchRateLimit(t *testing.T) {
	cli := NewClientWithOptions(ClientOptions{
		Burst:     3,
		PerSecond: 0.5,
	})
	limiter := cli.Limiter()
	if limiter == nil {
		t.Fatal("limiter is nil; ClientOptions ignored the burst config")
	}
	if got := limiter.Allow(); !got {
		t.Fatal("first Allow should return true")
	}
	if got := limiter.Allow(); !got {
		t.Fatal("second Allow should return true")
	}
	if got := limiter.Allow(); !got {
		t.Fatal("third Allow should return true (cap=3)")
	}
	if got := limiter.Allow(); got {
		t.Fatal("fourth Allow should return false (cap exhausted)")
	}
	if tokens := limiter.Tokens(); tokens != 0 {
		t.Errorf("tokens after burst exhaustion = %d, want 0", tokens)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	start := time.Now()
	if err := limiter.Wait(ctx); err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 1500*time.Millisecond {
		t.Errorf("Wait returned in %v; refill at 0.5/s means >2s expected", elapsed)
	}
	if elapsed > 3500*time.Millisecond {
		t.Errorf("Wait returned in %v; refill at 0.5/s means <3s expected", elapsed)
	}
}

func TestSearchCache(t *testing.T) {
	var htmlCalls atomic.Int32
	var robotsCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "robots.txt") {
			robotsCalls.Add(1)
			_, _ = w.Write([]byte(permissiveRobots()))
			return
		}
		htmlCalls.Add(1)
		_, _ = w.Write([]byte(ddgFixture))
	}))
	defer srv.Close()
	cli := NewClientWithOptions(ClientOptions{
		Endpoint: srv.URL,
		NoLimit:  true,
		CacheTTL: 5 * time.Minute,
	})
	for i := 0; i < 3; i++ {
		if _, err := cli.Search(context.Background(), "go programming language", 10); err != nil {
			t.Fatalf("iteration %d Search failed: %v", i, err)
		}
	}
	if htmlCalls.Load() != 1 {
		t.Errorf("expected exactly 1 html server call across 3 identical queries, got %d", htmlCalls.Load())
	}
	if robotsCalls.Load() > 1 {
		t.Errorf("expected at most 1 robots.txt server call (cached after first), got %d", robotsCalls.Load())
	}
	stats := cli.Cache().Stats()
	if stats.Hits < 2 {
		t.Errorf("expected >=2 cache hits on identical queries, got %d", stats.Hits)
	}
}

func TestSearchRobots(t *testing.T) {
	srv := newMockServer(t, bingFixtureHTML, "User-agent: *\nDisallow: /\n")
	defer srv.Close()
	cli := NewClientWithOptions(ClientOptions{
		Endpoint: srv.URL,
		NoLimit:  true,
	})
	_, err := cli.Search(context.Background(), "go", 10)
	if err == nil {
		t.Fatal("Search returned nil error; expected ErrDisallowed")
	}
	if err != ErrDisallowed {
		t.Fatalf("Search returned %v; want ErrDisallowed", err)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	cli := NewClient()
	_, err := cli.Search(context.Background(), "   ", 10)
	if err != ErrEmptyQuery {
		t.Fatalf("Search returned %v; want ErrEmptyQuery", err)
	}
}

func TestSearchCtxDeadline(t *testing.T) {
	srv := newMockServer(t, ddgFixture, permissiveRobots())
	defer srv.Close()
	rl := NewRateLimiter(1, 0.0001)
	cli := &Client{
		httpClient: srv.Client(),
		userAgent:  "test/1.0",
		endpoint:   srv.URL,
		maxResults: 10,
		cache:      NewCache(8, time.Minute),
		limiter:    rl,
	}
	for i := 0; i < 20; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		_, err := cli.Search(ctx, fmt.Sprintf("query-%d", i), 10)
		cancel()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Errorf("iteration %d: unexpected error %v", i, err)
		}
	}
}

func TestCacheRaceClean(t *testing.T) {
	cli := NewClient()
	cache := cli.Cache()
	const goroutines = 16
	const iterations = 64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				k := fmt.Sprintf("k-%d-%d", g, i%8)
				cache.Put(k, i)
				_, _ = cache.Get(k)
				_ = cache.Stats()
			}
		}(g)
	}
	wg.Wait()
	if cli.Cache().Stats().Size > cli.Cache().Stats().MaxSize {
		t.Fatal("cache exceeded MaxSize")
	}
}

func TestRateLimiterRaceClean(t *testing.T) {
	rl := NewRateLimiter(8, 200.0)
	const goroutines = 32
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var ok atomic.Int32
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if rl.Allow() {
					ok.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if ok.Load() == 0 {
		t.Fatal("rate limiter never admitted anything; the test burst ceiling is broken")
	}
}

func TestParseDuckDuckGoHTMLFn(t *testing.T) {
	rows := parseDuckDuckGoHTML(ddgFixture)
	if len(rows) != 3 {
		t.Fatalf("parseDuckDuckGoHTML rows=%d, want 3", len(rows))
	}
	if rows[0].URL != "https://golang.org/" {
		t.Errorf("rows[0].URL=%q, want unwrapped https://golang.org/", rows[0].URL)
	}
}

func TestUnduckURL(t *testing.T) {
	cases := map[string]string{
		`https://duckduckgo.com/l/?uddg=https%3A%2F%2Fgolang.org%2F&kl=us-en`: "https://golang.org/",
		`https://example.com/3`:           "https://example.com/3",
		``:                                "",
		`https://duckduckgo.com/l/?uddg=`: "https://duckduckgo.com/l/?uddg=",
	}
	for in, want := range cases {
		if got := unduckURL(in); got != want {
			t.Errorf("unduckURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseRobots(t *testing.T) {
	body := "# top comment\nUser-agent: *\nDisallow: /foo\nDisallow: /bar/baz\n  \nDisallow: \nAllow: /anything\n"
	got := parseRobots(body)
	want := []string{"/foo", "/bar/baz", ""}
	if len(got) != len(want) {
		t.Fatalf("parseRobots length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseRobots[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
