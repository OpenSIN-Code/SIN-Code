// SPDX-License-Identifier: MIT
// Purpose: tests for the native browser-automation foundation (issue #382).
// Drives the public surface — Browser, Page, Transport — through the
// stub transport so callers can verify behavior independent of any
// real Chromium install. Race-clean.
package browser

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Transport stub helpers -----------------------------------------------

// recordingTransport captures every call so tests can assert on the
// exact request shape.
type recordingTransport struct {
	mu       sync.Mutex
	fetchReq []Request
	renderReq []renderCall
	closed   bool
	renderFn func(ctx context.Context, url string, opts RenderOpts) (RenderResult, error)
	fetchFn  func(ctx context.Context, req Request) (Response, error)
}

type renderCall struct {
	url  string
	opts RenderOpts
}

func (r *recordingTransport) Name() string { return "recording" }
func (r *recordingTransport) Fetch(_ context.Context, req Request) (Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fetchReq = append(r.fetchReq, req)
	if r.fetchFn != nil {
		return r.fetchFn(context.Background(), req)
	}
	return Response{StatusCode: 200, Body: []byte("ok"), Duration: time.Millisecond}, nil
}
func (r *recordingTransport) Render(_ context.Context, url string, opts RenderOpts) (RenderResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.renderReq = append(r.renderReq, renderCall{url: url, opts: opts})
	if r.renderFn != nil {
		return r.renderFn(context.Background(), url, opts)
	}
	return RenderResult{
		Data: []byte("stub-img"), Format: opts.Format,
		Width: 1280, Height: 720, URL: url, Duration: time.Millisecond,
	}, nil
}
func (r *recordingTransport) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

func (r *recordingTransport) calls() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.fetchReq), len(r.renderReq)
}

// httpTransport wraps an httptest.Server's URL space as a Transport.
// Lets tests verify that the production HTTP transport path works for
// Fetch even when the rest of the package stays stubbed.
type httpTransport struct {
	baseURL string
	client  *http.Client // optional test override
}

func (h *httpTransport) Name() string { return "http" }
func (h *httpTransport) Fetch(_ context.Context, req Request) (Response, error) {
	start := time.Now()
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if len(req.Body) > 0 {
		body = strings.NewReader(string(req.Body))
	}
	r, err := http.NewRequest(method, req.URL, body)
	if err != nil {
		return Response{}, err
	}
	for k, v := range req.Headers {
		r.Header.Set(k, v)
	}
	client := h.client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(r)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	hdrs := map[string]string{}
	for k, vs := range resp.Header {
		if len(vs) > 0 {
			hdrs[k] = vs[0]
		}
	}
	return Response{
		StatusCode: resp.StatusCode,
		Headers:    hdrs,
		Body:       buf,
		Duration:   time.Since(start),
	}, nil
}
func (h *httpTransport) Render(_ context.Context, url string, _ RenderOpts) (RenderResult, error) {
	// Render stub — return a header + URL marker.
	start := time.Now()
	hdr := pngStubHeader(640, 360)
	return RenderResult{
		Data:     append(hdr[:], []byte(url)...),
		Format:   "png",
		Width:    640, Height: 360, URL: url,
		Duration: time.Since(start),
	}, nil
}
func (h *httpTransport) Close() error { return nil }

// --- tests -----------------------------------------------------------------

func TestBrowser_NewPage_Fetch(t *testing.T) {
	tr := &recordingTransport{}
	b := NewBrowser(Config{Transport: tr})
	defer b.Close()
	page, err := b.NewPage()
	if err != nil {
		t.Fatalf("NewPage: %v", err)
	}
	defer page.Close()
	res, err := b.Fetch(context.Background(), Request{
		URL: "http://example.test", Method: http.MethodGet,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	if got := string(res.Body); got != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", got)
	}
	fc, _ := tr.calls()
	if fc != 1 {
		t.Fatalf("expected 1 fetch, got %d", fc)
	}
}

func TestBrowser_Fetch_EmptyURL(t *testing.T) {
	b := NewBrowser(Config{})
	defer b.Close()
	if _, err := b.Fetch(context.Background(), Request{}); err == nil {
		t.Fatal("expected error on empty URL")
	}
}

func TestBrowser_Render_DefaultStub(t *testing.T) {
	b := NewBrowser(Config{})
	defer b.Close()
	r, err := b.Render(context.Background(), "https://x", RenderOpts{})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(r.Data) == 0 {
		t.Fatal("expected data, got empty")
	}
	if r.Format != "png" {
		t.Fatalf("expected png format, got %q", r.Format)
	}
	if r.Width != 1280 || r.Height != 720 {
		t.Fatalf("expected 1280x720, got %dx%d", r.Width, r.Height)
	}
	if !bytesHasPrefix(r.Data, []byte{0x89, 0x50, 0x4E, 0x47}) {
		t.Fatal("render output missing PNG signature")
	}
}

func TestBrowser_Render_ViewportParsing(t *testing.T) {
	b := NewBrowser(Config{})
	defer b.Close()
	r, err := b.Render(context.Background(), "https://x", RenderOpts{Viewport: "800x600"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if r.Width != 800 || r.Height != 600 {
		t.Fatalf("expected 800x600, got %dx%d", r.Width, r.Height)
	}
}

func TestBrowser_RenderEmptyURL(t *testing.T) {
	b := NewBrowser(Config{})
	defer b.Close()
	_, err := b.Render(context.Background(), "", RenderOpts{})
	if err == nil {
		t.Fatal("expected error on empty url")
	}
}

func TestBrowser_PageLifecycle(t *testing.T) {
	tr := &recordingTransport{}
	b := NewBrowser(Config{Transport: tr})
	defer b.Close()
	p1, err := b.NewPage()
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	p2, err := b.NewPage()
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(b.Pages()) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(b.Pages()))
	}
	if _, err := p1.Navigate(context.Background(), "https://a"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if p1.URL() != "https://a" {
		t.Fatalf("expected URL https://a, got %q", p1.URL())
	}
	p1.SetTitle("hello")
	if p1.Title() != "hello" {
		t.Fatalf("expected title hello, got %q", p1.Title())
	}
	if err := p1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := p1.Navigate(context.Background(), "https://b"); err == nil {
		t.Fatal("expected error after page close")
	}
	if err := p1.Close(); err != nil {
		t.Fatalf("second Close should be idempotent: %v", err)
	}
	if len(b.Pages()) != 2 {
		t.Fatalf("expected 2 pages still registered, got %d", len(b.Pages()))
	}
	_ = p2
}

func TestBrowser_PageNavigate_EmptyURL(t *testing.T) {
	tr := &recordingTransport{}
	b := NewBrowser(Config{Transport: tr})
	defer b.Close()
	p, _ := b.NewPage()
	defer p.Close()
	if _, err := p.Navigate(context.Background(), ""); err == nil {
		t.Fatal("expected error on empty url")
	}
}

func TestBrowser_CloseCascadesToTransport(t *testing.T) {
	tr := &recordingTransport{}
	b := NewBrowser(Config{Transport: tr})
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !tr.closed {
		t.Fatal("transport should be closed")
	}
	if err := b.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestBrowser_ClosedRejectsAllCalls(t *testing.T) {
	tr := &recordingTransport{}
	b := NewBrowser(Config{Transport: tr})
	_ = b.Close()
	if _, err := b.Fetch(context.Background(), Request{URL: "x"}); err == nil {
		t.Fatal("expected error from closed browser")
	}
	if _, err := b.Render(context.Background(), "x", RenderOpts{}); err == nil {
		t.Fatal("expected error from closed browser")
	}
	if _, err := b.NewPage(); err == nil {
		t.Fatal("expected error from closed browser")
	}
}

func TestBrowser_ConversationTracing(t *testing.T) {
	var calls atomic.Int32
	b := NewBrowser(Config{
		TraceHook: func(_ string) { calls.Add(1) },
	})
	defer b.Close()
	_, _ = b.Fetch(context.Background(), Request{URL: "http://x"})
	_, _ = b.Render(context.Background(), "http://x", RenderOpts{})
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 trace calls, got %d", got)
	}
}

func TestStubTransport_Override(t *testing.T) {
	st := NewStubTransport()
	st.Override().Set("http://override.test", 418, "teapot")
	resp, err := st.Fetch(context.Background(), Request{URL: "http://override.test"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 418 || string(resp.Body) != "teapot" {
		t.Fatalf("unexpected: status=%d body=%q", resp.StatusCode, resp.Body)
	}
}

func TestStubTransport_DefaultPattern(t *testing.T) {
	st := NewStubTransport()
	resp, err := st.Fetch(context.Background(), Request{URL: "http://no-override.test"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 default, got %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "http://no-override.test") {
		t.Fatalf("expected stub body to embed URL, got %q", resp.Body)
	}
}

func TestStubTransport_DefaultFallback(t *testing.T) {
	st := NewStubTransport()
	st.Override().SetDefault(503, "down")
	resp, err := st.Fetch(context.Background(), Request{URL: "http://anything.test"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 503 || string(resp.Body) != "down" {
		t.Fatalf("expected default fallback, got status=%d body=%q", resp.StatusCode, resp.Body)
	}
}

func TestStubTransport_FetchEmptyURL(t *testing.T) {
	st := NewStubTransport()
	if _, err := st.Fetch(context.Background(), Request{}); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestStubTransport_RenderEmpty(t *testing.T) {
	st := NewStubTransport()
	if _, err := st.Render(context.Background(), "", RenderOpts{}); err == nil {
		t.Fatal("expected error for empty url")
	}
}

func TestBrowser_FetchViaRealHTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Probe") != "1" {
			t.Errorf("expected X-Probe=1, got %q", r.Header.Get("X-Probe"))
		}
		w.Header().Set("X-Back", "ok")
		_, _ = w.Write([]byte("hello from test server"))
	}))
	defer srv.Close()
	tr := &httpTransport{
		baseURL: srv.URL,
		client:  &http.Client{Timeout: 2 * time.Second},
	}
	b := NewBrowser(Config{Transport: tr, UserAgent: "sin-test/1.0"})
	defer b.Close()
	resp, err := b.Fetch(context.Background(), Request{
		URL: srv.URL + "/whatever", Method: "POST",
		Headers: map[string]string{"X-Probe": "1"},
		Body:    []byte("hello"),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if string(resp.Body) != "hello from test server" {
		t.Fatalf("unexpected body: %q", resp.Body)
	}
	if resp.Headers["X-Back"] != "ok" {
		t.Fatalf("expected X-Back=ok, got %q", resp.Headers["X-Back"])
	}
	// Round-trip the response back through tests for sanity.
	if resp.Duration <= 0 {
		t.Fatal("expected positive duration")
	}
}

func TestBrowser_TransportName(t *testing.T) {
	st := NewStubTransport()
	b := NewBrowser(Config{Transport: st})
	defer b.Close()
	if got := b.TransportName(); got != "stub" {
		t.Fatalf("expected stub, got %q", got)
	}
	tr := &recordingTransport{}
	b2 := NewBrowser(Config{Transport: tr})
	defer b2.Close()
	if got := b2.TransportName(); got != "recording" {
		t.Fatalf("expected recording, got %q", got)
	}
}

func TestBrowser_MultipleConcurrentFetches(t *testing.T) {
	tr := &recordingTransport{
		fetchFn: func(_ context.Context, _ Request) (Response, error) {
			time.Sleep(20 * time.Millisecond) // simulate I/O
			return Response{StatusCode: 200, Body: []byte("ok")}, nil
		},
	}
	b := NewBrowser(Config{Transport: tr})
	defer b.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := b.Fetch(context.Background(), Request{URL: "http://x"}); err != nil {
				t.Errorf("Fetch: %v", err)
			}
		}()
	}
	wg.Wait()
	fc, _ := tr.calls()
	if fc != 8 {
		t.Fatalf("expected 8 fetches, got %d", fc)
	}
}

func TestBrowser_FetchMethodDefaultsToGET(t *testing.T) {
	tr := &recordingTransport{}
	b := NewBrowser(Config{Transport: tr})
	defer b.Close()
	_, _ = b.Fetch(context.Background(), Request{URL: "http://x"})
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.fetchReq[0].Method != "GET" {
		t.Fatalf("expected default GET, got %q", tr.fetchReq[0].Method)
	}
}

func TestBrowser_NavigateEmptyURL_ErrorsAtEngine(t *testing.T) {
	tr := &recordingTransport{
		renderFn: func(_ context.Context, url string, _ RenderOpts) (RenderResult, error) {
			return RenderResult{}, errors.New("should not happen")
		},
	}
	b := NewBrowser(Config{Transport: tr})
	defer b.Close()
	p, _ := b.NewPage()
	if _, err := p.Navigate(context.Background(), ""); err == nil {
		t.Fatal("expected error on empty url")
	}
}

func TestBrowser_TraceHookReceivesRealPayload(t *testing.T) {
	var seen []string
	var mu sync.Mutex
	b := NewBrowser(Config{
		TraceHook: func(s string) {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, s)
		},
	})
	defer b.Close()
	_, _ = b.Fetch(context.Background(), Request{URL: "http://x", Method: "GET"})
	_, _ = b.Render(context.Background(), "http://y", RenderOpts{})
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("expected 2 trace events, got %v", seen)
	}
	if !strings.Contains(seen[0], "fetch:GET:") || !strings.Contains(seen[1], "render:http://y:") {
		t.Fatalf("trace payload malformed: %v", seen)
	}
}

// helper — bytes.HasPrefix works for byte slices; avoid importing bytes in
// tests when engine_test.go does not need it.
func bytesHasPrefix(b, prefix []byte) bool {
	if len(prefix) > len(b) {
		return false
	}
	for i, c := range prefix {
		if b[i] != c {
			return false
		}
	}
	return true
}
