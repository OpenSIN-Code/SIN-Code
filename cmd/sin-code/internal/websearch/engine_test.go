// SPDX-License-Identifier: MIT
// Purpose: tests for the native web-search engine (issue #381). Drives
// the engine and every provider through an httptest.Server so the
// production code path is exercised end-to-end. Race-clean.
package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- fakeDoer --------------------------------------------------------------

// fakeDoer is the test HTTPDoer. Routes a request to a response based on
// matcher registration order; first match wins. Lets us drive every
// provider through its real HTTP shape without spinning up a unique
// httptest.Server per provider.
type fakeDoer struct {
	mu      atomic.Int32
	matches []fakeMatch
}

type fakeMatch struct {
	method  string
	urlSub  string
	status  int
	body    string
	content string
}

func (f *fakeDoer) Do(req *httpRequest) (*httpResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("fakeDoer: nil request")
	}
	for _, m := range f.matches {
		if m.method != "" && m.method != req.method {
			continue
		}
		if m.urlSub != "" && !strings.Contains(req.url, m.urlSub) {
			continue
		}
		hdr := http.Header{}
		if m.content != "" {
			hdr.Set("Content-Type", m.content)
		}
		return &httpResponse{
			status: m.status,
			header: hdr,
			body:   []byte(m.body),
		}, nil
	}
	return &httpResponse{
		status: 404,
		header: http.Header{},
		body:   []byte(`{"error":"no fake route registered"}`),
	}, nil
}

func (f *fakeDoer) register(m fakeMatch) {
	f.matches = append(f.matches, m)
}

// helper: convert real httptest.Server URLs into a fakeDoer fixture so
// tests can drive exact payload assertions against the production code
// path (no rewriting).
func wireFakeRoutesDoer(server *httptest.Server, ask *atomicCounter) HTTPDoer {
	return &redirectDoer{
		server: server.URL,
		ask:    ask,
	}
}

// atomicCounter is a tiny test helper around int32.
type atomicCounter struct {
	v atomic.Int32
}

func (a *atomicCounter) inc() { a.v.Add(1) }
func (a *atomicCounter) get() int32 { return a.v.Load() }

// redirectDoer redirects an httpRequest's URL prefix to the test server
// and otherwise delegates to fakeDoer-style logic. Kept distinct so
// tests can decide whether they want full HTTP roundtrip (redirectDoer)
// or pure stub (fakeDoer).
type redirectDoer struct {
	server string
	ask    *atomicCounter
}

func (r *redirectDoer) Do(req *httpRequest) (*httpResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("redirectDoer: nil request")
	}
	if r.ask != nil {
		r.ask.inc()
	}
	u, err := url.Parse(req.url)
	if err != nil {
		return nil, fmt.Errorf("redirectDoer: parse %q: %w", req.url, err)
	}
	realURL := r.server + u.RequestURI()
	method := req.method
	var body []byte
	switch method {
	case http.MethodPost:
		body = req.body
	}
	r2, err := http.NewRequest(method, realURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	if req.header != nil {
		r2.Header = req.header.Clone()
	}
	resp, err := http.DefaultClient.Do(r2)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	buf := make([]byte, 0, 1024)
	chunk := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			break
		}
	}
	return &httpResponse{
		status: resp.StatusCode,
		header: resp.Header.Clone(),
		body:   buf,
	}, nil
}

// --- tests ----------------------------------------------------------------

func TestEngine_EmptyQuery(t *testing.T) {
	e := NewEngine(LoadConfig())
	_, err := e.Search(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	if !strings.Contains(err.Error(), "empty query") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngine_NoProviders(t *testing.T) {
	e := &Engine{cfg: LoadConfig()}
	_, err := e.Search(context.Background(), "golang")
	if err == nil {
		t.Fatal("expected error for zero providers")
	}
	if !strings.Contains(err.Error(), "no providers registered") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngine_NoActiveProvidersNoResults(t *testing.T) {
	// Build an engine where every provider is a stub returning no rows.
	// DuckDuckGo in a real config has no key gate and would try a real HTTP
	// call — override its doer with a fake so the no-results branch is the
	// only possible outcome.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`<html><body></body></html>`))
	}))
	defer srv.Close()
	e := &Engine{cfg: Config{HTTPClient: &redirectDoer{server: srv.URL}}}
	e.AddProvider(NewSerpAPIProvider(Config{HTTPClient: e.cfg.HTTPClient}))
	e.AddProvider(NewBraveProvider(Config{HTTPClient: e.cfg.HTTPClient}))
	e.AddProvider(NewBingProvider(Config{HTTPClient: e.cfg.HTTPClient}))
	e.AddProvider(NewDuckDuckGoProvider(Config{HTTPClient: e.cfg.HTTPClient}))
	e.AddProvider(NewTavilyProvider(Config{HTTPClient: e.cfg.HTTPClient}))
	_, err := e.Search(context.Background(), "rust")
	if err == nil {
		t.Fatal("expected error when no provider can return rows")
	}
	if !strings.Contains(err.Error(), "no results from any active provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEngine_AddProviderNilAndDuplicates(t *testing.T) {
	e := &Engine{cfg: LoadConfig()}
	e.AddProvider(nil)
	if len(e.Providers()) != 0 {
		t.Fatal("nil provider should be ignored")
	}
	fake := &testProvider{name: "x"}
	e.AddProvider(fake)
	e.AddProvider(fake) // duplicate
	e.AddProvider(&testProvider{name: "x"}) // same name, different pointer
	if got := len(e.Providers()); got != 1 {
		t.Fatalf("expected 1 unique provider, got %d", got)
	}
}

type testProvider struct {
	name string
	rows []Result
	err  error
}

func (t *testProvider) Name() string { return t.name }
func (t *testProvider) Search(_ context.Context, _ string) ([]Result, error) {
	return t.rows, t.err
}

func TestEngine_DedupAndRank(t *testing.T) {
	e := &Engine{cfg: LoadConfig()}
	e.AddProvider(&testProvider{
		name: "p1",
		rows: []Result{
			{Title: "first", URL: "https://a", Score: 0.7},
			{Title: "second", URL: "https://b", Score: 0.3},
		},
	})
	e.AddProvider(&testProvider{
		name: "p2",
		rows: []Result{
			{Title: "dup", URL: "https://a", Score: 0.9},
			{Title: "third", URL: "https://c", Score: 0.5},
		},
	})
	rows, err := e.Search(context.Background(), "anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 unique rows, got %d", len(rows))
	}
	if rows[0].URL != "https://a" || rows[0].Score != 0.7 {
		t.Fatalf("first row should be https://a with kept score 0.7, got %+v", rows[0])
	}
	if rows[1].Score < rows[2].Score {
		t.Fatalf("rows should be sorted by score desc, got %+v", rows)
	}
}

func TestEngine_FailsClosedOnProviderError(t *testing.T) {
	e := &Engine{cfg: LoadConfig()}
	e.AddProvider(&testProvider{name: "good", rows: []Result{
		{Title: "ok", URL: "https://ok", Score: 0.4},
	}})
	e.AddProvider(&testProvider{
		name: "bad",
		err:  fmt.Errorf("provider exploded"),
		rows: nil,
	})
	rows, err := e.Search(context.Background(), "k")
	if err != nil {
		t.Fatalf("search should ignore failing provider: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected only the good provider's row, got %d", len(rows))
	}
}

func TestEngine_AllProvidersFailing(t *testing.T) {
	e := &Engine{cfg: LoadConfig()}
	e.AddProvider(&testProvider{name: "p1", err: fmt.Errorf("x")})
	e.AddProvider(&testProvider{name: "p2", err: fmt.Errorf("y")})
	_, err := e.Search(context.Background(), "k")
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestEngine_StatsSorted(t *testing.T) {
	e := &Engine{cfg: LoadConfig()}
	e.AddProvider(&testProvider{name: "z"})
	e.AddProvider(&testProvider{name: "a"})
	e.AddProvider(&testProvider{name: "m"})
	s := e.Stats()
	want := []string{"a", "m", "z"}
	if len(s.ActiveNames) != len(want) {
		t.Fatalf("expected %d names, got %d", len(want), len(s.ActiveNames))
	}
	for i := range want {
		if s.ActiveNames[i] != want[i] {
			t.Fatalf("name %d: want %q got %q", i, want[i], s.ActiveNames[i])
		}
	}
	if s.Providers != 3 {
		t.Fatalf("expected providers=3, got %d", s.Providers)
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	e := &Engine{cfg: Config{Timeout: 5 * time.Second}}
	// slowProvider sleeps until ctx is done.
	slow := &blockingProvider{}
	e.AddProvider(slow)
	// fastProvider returns immediately
	e.AddProvider(&testProvider{name: "fast", rows: []Result{
		{Title: "fast", URL: "https://fast", Score: 0.9},
	}})
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	rows, err := e.Search(ctx, "k")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row from fast provider, got %d", len(rows))
	}
	// The fast provider should have returned well before the 5s slow-provider deadline.
	if elapsed > 2*time.Second {
		t.Fatalf("search took too long (%v); probably blocked on slow provider", elapsed)
	}
}

type blockingProvider struct{}

func (b *blockingProvider) Name() string { return "blocking" }
func (b *blockingProvider) Search(ctx context.Context, _ string) ([]Result, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestEngine_CancelledContext(t *testing.T) {
	e := &Engine{cfg: Config{Timeout: 30 * time.Second}}
	e.AddProvider(&blockingProvider{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Search(ctx, "k"); err == nil {
		t.Fatal("expected error when context already cancelled")
	}
}

// --- provider HTTP tests (mock servers) -----------------------------------

func TestSerpAPIProvider_NoKey(t *testing.T) {
	p := NewSerpAPIProvider(Config{})
	rows, err := p.Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for missing key, got %d", len(rows))
	}
}

func TestSerpAPIProvider_HappyPath_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "k1" {
			t.Errorf("expected api_key=k1, got %q", r.URL.Query().Get("api_key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"organic_results": []map[string]any{
				{"title": "T1", "link": "https://x", "snippet": "S1", "position": 1},
				{"title": "T2", "link": "https://y", "snippet": "S2", "position": 2},
			},
		})
	}))
	defer srv.Close()
	cfg := Config{
		SerpAPIKey: "k1",
		HTTPClient: &redirectDoer{server: srv.URL},
	}
	p := NewSerpAPIProvider(cfg)
	rows, err := p.Search(context.Background(), "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].URL != "https://x" {
		t.Fatalf("expected first URL https://x, got %q", rows[0].URL)
	}
	if rows[0].Score <= rows[1].Score {
		t.Fatalf("expected position-based descending scores, got %v, %v", rows[0].Score, rows[1].Score)
	}
}

func TestSerpAPIProvider_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"server"}`))
	}))
	defer srv.Close()
	cfg := Config{SerpAPIKey: "k", HTTPClient: &redirectDoer{server: srv.URL}}
	p := NewSerpAPIProvider(cfg)
	_, err := p.Search(context.Background(), "foo")
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("expected status 500, got %v", err)
	}
}

func TestBraveProvider_HappyPath_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "br" {
			t.Errorf("expected X-Subscription-Token=br, got %q", r.Header.Get("X-Subscription-Token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"web":{"results":[{"title":"BT","url":"https://brave","description":"BD"}]}}`))
	}))
	defer srv.Close()
	cfg := Config{BraveKey: "br", HTTPClient: &redirectDoer{server: srv.URL}}
	p := NewBraveProvider(cfg)
	rows, err := p.Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || rows[0].URL != "https://brave" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestBingProvider_HappyPath_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Ocp-Apim-Subscription-Key") != "bk" {
			t.Errorf("expected Ocp-Apim-Subscription-Key=bk, got %q", r.Header.Get("Ocp-Apim-Subscription-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webPages":{"value":[{"name":"BN","url":"https://bing","snippet":"BS"}]}}`))
	}))
	defer srv.Close()
	cfg := Config{BingKey: "bk", HTTPClient: &redirectDoer{server: srv.URL}}
	p := NewBingProvider(cfg)
	rows, err := p.Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || rows[0].URL != "https://bing" {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestTavilyProvider_HappyPath_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"TT","url":"https://tav","content":"TD","score":0.42}]}`))
	}))
	defer srv.Close()
	cfg := Config{TavilyKey: "tk", HTTPClient: &redirectDoer{server: srv.URL}}
	p := NewTavilyProvider(cfg)
	rows, err := p.Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 || rows[0].Score != 0.42 {
		t.Fatalf("unexpected rows: %+v", rows)
	}
}

func TestDuckDuckGoProvider_ParsesHTML(t *testing.T) {
	// Hand-built HTML shaped like html.duckduckgo.com/html/ output.
	body := `<html><body>
<a rel="nofollow" class="result__a" href="https://duck/l/?uddg=https%3A%2F%2Freal1.example">RealOne</a>
<a class="result__snippet" href="">Snippet one text.</a>
<a rel="nofollow" class="result__a" href="https://duck/l/?uddg=https%3A%2F%2Freal2.example">RealTwo</a>
<a class="result__snippet" href="">Snippet two text.</a>
</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	cfg := Config{HTTPClient: &redirectDoer{server: srv.URL}}
	p := NewDuckDuckGoProvider(cfg)
	rows, err := p.Search(context.Background(), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].URL != "https://real1.example" {
		t.Fatalf("expected unducked URL, got %q", rows[0].URL)
	}
	if rows[0].Title != "RealOne" {
		t.Fatalf("expected title RealOne, got %q", rows[0].Title)
	}
	if rows[0].Snippet != "Snippet one text." {
		t.Fatalf("expected snippet text, got %q", rows[0].Snippet)
	}
}

func TestUnduckURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://duck/l/?uddg=https%3A%2F%2Freal", "https://real"},
		{"https://real/path", "https://real/path"},
		{"", ""},
	}
	for i, c := range cases {
		if got := unduckURL(c.in); got != c.want {
			t.Errorf("case %d: unduckURL(%q)=%q, want %q", i, c.in, got, c.want)
		}
	}
}

func TestEngine_AllFiveProvidersWiredUp(t *testing.T) {
	e := NewEngine(LoadConfig())
	names := e.Stats().ActiveNames
	want := map[string]bool{
		"brave": false, "bing": false, "duckduckgo": false,
		"serpapi": false, "tavily": false,
	}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for n, seen := range want {
		if !seen {
			t.Errorf("provider %q missing from NewEngine wiring", n)
		}
	}
}

func TestLoadConfig_ReadsEnv(t *testing.T) {
	t.Setenv("WEBSEARCH_SERPAPI_KEY", "abc")
	t.Setenv("WEBSEARCH_BRAVE_KEY", "def")
	t.Setenv("WEBSEARCH_BING_KEY", "ghi")
	t.Setenv("WEBSEARCH_TAVILY_KEY", "jkl")
	cfg := LoadConfig()
	if cfg.SerpAPIKey != "abc" || cfg.BraveKey != "def" || cfg.BingKey != "ghi" || cfg.TavilyKey != "jkl" {
		t.Fatalf("env vars not honored: %+v", cfg)
	}
	if cfg.UserAgent == "" {
		t.Fatal("expected non-empty user agent")
	}
}

func TestLoadConfig_MissingEnv(t *testing.T) {
	os.Unsetenv("WEBSEARCH_SERPAPI_KEY")
	os.Unsetenv("WEBSEARCH_BRAVE_KEY")
	os.Unsetenv("WEBSEARCH_BING_KEY")
	os.Unsetenv("WEBSEARCH_TAVILY_KEY")
	cfg := LoadConfig()
	if cfg.SerpAPIKey != "" || cfg.BraveKey != "" || cfg.BingKey != "" || cfg.TavilyKey != "" {
		t.Fatalf("expected empty keys, got %+v", cfg)
	}
}

func TestEngine_MultipleProvidersDeDupByURL(t *testing.T) {
	e := &Engine{cfg: LoadConfig()}
	// Three providers, all return the SAME URL with different scores — first win, dedupe keeps the score from p1.
	e.AddProvider(&testProvider{name: "p1", rows: []Result{
		{Title: "p1", URL: "https://dup", Score: 0.9},
	}})
	e.AddProvider(&testProvider{name: "p2", rows: []Result{
		{Title: "p2", URL: "https://dup", Score: 0.5},
	}})
	e.AddProvider(&testProvider{name: "p3", rows: []Result{
		{Title: "p3", URL: "https://dup", Score: 0.1},
	}})
	rows, err := e.Search(context.Background(), "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 unique row, got %d", len(rows))
	}
	if rows[0].Score != 0.9 || rows[0].Title != "p1" {
		t.Fatalf("expected first-writer-wins with p1/0.9, got %+v", rows[0])
	}
}

func TestProviderInterface_Serializability(t *testing.T) {
	// Compile-time guard that every provider satisfies Provider.
	var _ Provider = (*SerpAPIProvider)(nil)
	var _ Provider = (*BraveProvider)(nil)
	var _ Provider = (*BingProvider)(nil)
	var _ Provider = (*DuckDuckGoProvider)(nil)
	var _ Provider = (*TavilyProvider)(nil)
	var _ Provider = (*testProvider)(nil)
}

func TestEngine_NewEngineDefaultWiring_NoKeys(t *testing.T) {
	e := NewEngine(Config{})
	if e == nil {
		t.Fatal("NewEngine must not return nil")
	}
	if e.Stats().Providers != 5 {
		t.Fatalf("expected 5 providers (all gated), got %d", e.Stats().Providers)
	}
}
