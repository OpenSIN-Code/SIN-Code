// SPDX-License-Identifier: MIT
// Purpose: stub-based browser-automation foundation (issue #382).
// Defines the public surface — Browser, Page, Request, Response,
// RenderOpts, RenderResult — and a Transport interface so callers can
// plug in either an in-process stub (default), a child-process shim
// (`chromedp` / `playwright`) or a remote-WebSocket runtime later.
// Built on top of the autonomy package because the headline use case
// (issue #382 title) is "Autodev/Autoresearch: Native browser
// automation in Go binary" — autonomous research workflows.
package browser

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Request is one outbound HTTP exchange. Mirrors http.Request at the
// surface but is decoupled so the in-process Transport can stub any
// combination of method/headers/body without pulling net/http into the
// hot path.
type Request struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    []byte
	Timeout time.Duration
}

// Response is the result of one Fetch. Duration measures wall-clock
// from Transport.Do start to response complete.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	Duration   time.Duration
}

// RenderOpts tunes a RenderResult. Viewport is "WxH" (default
// 1280x720); FullPage asks the backend for a full-page screenshot instead
// of viewport.
type RenderOpts struct {
	Viewport string
	FullPage bool
	Timeout  time.Duration
	Format   string // "png" | "jpeg" (default "png")
}

// RenderResult is the screenshot / rendered output.
type RenderResult struct {
	Data     []byte
	Format   string
	Width    int
	Height   int
	URL      string
	Duration time.Duration
}

// Transport is the pluggable browser backend contract. The default
// implementation (stubTransport) returns deterministic stub output so
// callers can use the package without spinning a real browser. Tests
// inject custom Transports to drive every code path.
type Transport interface {
	Fetch(ctx context.Context, req Request) (Response, error)
	Render(ctx context.Context, url string, opts RenderOpts) (RenderResult, error)
	Close() error
	Name() string // "stub" | "http" | "chromedp" | "playwright"
}

// Page is one logical browser tab. Pages are cheap to create; each
// Page holds its own Transport reference so concurrent Pages can use
// different Transports if needed.
type Page struct {
	mu       sync.Mutex
	url      string
	parent   *Browser
	closed   bool
	title    string
}

// Page returns the current URL the page is showing.
func (p *Page) URL() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.url
}

// Title returns the page title captured by SetTitle (default empty).
func (p *Page) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.title
}

// Navigate moves the page to a new URL through the transport. Returns
// the response from the transport's Render call.
func (p *Page) Navigate(ctx context.Context, url string) (RenderResult, error) {
	if p == nil {
		return RenderResult{}, errors.New("browser: nil page")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return RenderResult{}, errors.New("browser: page is closed")
	}
	if url == "" {
		return RenderResult{}, errors.New("browser: empty url")
	}
	res, err := p.parent.transport.Render(ctx, url, RenderOpts{})
	if err != nil {
		return RenderResult{}, fmt.Errorf("browser: navigate: %w", err)
	}
	p.url = url
	return res, nil
}

// SetTitle is exposed so callers can mirror the page title from their
// own webdriver / scraper integration when the Transport does not
// surface one.
func (p *Page) SetTitle(title string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.title = title
}

// Close releases the page. After Close, the Page refuses all subsequent
// calls with a descriptive error.
func (p *Page) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return nil
}

// Browser is the top-level entry point. Holds one Transport and a set
// of Pages. Safe for concurrent use.
type Browser struct {
	mu         sync.RWMutex
	transport  Transport
	pages      []*Page
	cfg        Config
	closed     bool
}

// Config configures a Browser. Transport may be nil (defaults to the
// in-process stub). UserAgent is sent on every Fetch header set.
type Config struct {
	UserAgent string
	Transport Transport
	// TraceHook, when non-nil, fires after every Fetch/Render call so
	// the agentloop can stream browser events into the lessons DB.
	TraceHook func(event string)
}

// NewBrowser returns a Browser wired up against cfg.Transport (or
// the in-process stub if nil).
func NewBrowser(cfg Config) *Browser {
	tr := cfg.Transport
	if tr == nil {
		tr = NewStubTransport()
	}
	return &Browser{
		transport: tr,
		cfg:       cfg,
	}
}

// NewPage opens a new Page and registers it with the Browser. Pages
// remain in the Browser's slice until Close is called on them.
func (b *Browser) NewPage() (*Page, error) {
	if b == nil {
		return nil, errors.New("browser: nil browser")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("browser: closed")
	}
	p := &Page{parent: b}
	b.pages = append(b.pages, p)
	return p, nil
}

// Pages returns a snapshot of the pages list.
func (b *Browser) Pages() []*Page {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]*Page, len(b.pages))
	copy(out, b.pages)
	return out
}

// Fetch issues a raw HTTP request through the transport. The Browser
// itself does not retain request state — for full browser semantics,
// open a Page and use Page.Navigate.
func (b *Browser) Fetch(ctx context.Context, req Request) (Response, error) {
	if b == nil {
		return Response{}, errors.New("browser: nil browser")
	}
	b.mu.RLock()
	tr := b.transport
	cfg := b.cfg
	closed := b.closed
	b.mu.RUnlock()
	if closed {
		return Response{}, errors.New("browser: closed")
	}
	if req.URL == "" {
		return Response{}, errors.New("browser: empty url")
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if cfg.UserAgent != "" && req.Headers == nil {
		req.Headers = map[string]string{}
	}
	if req.Headers != nil && cfg.UserAgent != "" {
		if _, ok := req.Headers["User-Agent"]; !ok {
			req.Headers["User-Agent"] = cfg.UserAgent
		}
	}
	res, err := tr.Fetch(ctx, req)
	if cfg.TraceHook != nil {
		cfg.TraceHook(fmt.Sprintf("fetch:%s:%d", req.Method, res.StatusCode))
	}
	return res, err
}

// Render captures a screenshot (or pdf/html dump depending on the
// transport) of the URL. The result type is intentionally permissive:
// callers that only need screenshots inspect RenderResult.Format ==
// "png".
func (b *Browser) Render(ctx context.Context, url string, opts RenderOpts) (RenderResult, error) {
	if b == nil {
		return RenderResult{}, errors.New("browser: nil browser")
	}
	b.mu.RLock()
	tr := b.transport
	cfg := b.cfg
	closed := b.closed
	b.mu.RUnlock()
	if closed {
		return RenderResult{}, errors.New("browser: closed")
	}
	if url == "" {
		return RenderResult{}, errors.New("browser: empty url")
	}
	res, err := tr.Render(ctx, url, opts)
	if cfg.TraceHook != nil {
		cfg.TraceHook(fmt.Sprintf("render:%s:%d", url, len(res.Data)))
	}
	return res, err
}

// Close shuts down the underlying transport and marks the Browser as
// closed. The Transport's Close is propagated.
func (b *Browser) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	tr := b.transport
	for _, p := range b.pages {
		_ = p.Close()
	}
	b.pages = nil
	if tr != nil {
		return tr.Close()
	}
	return nil
}

// TransportName returns the wrapped Transport's identifier. Useful
// for telemetry ("stub" vs "chromedp" etc.).
func (b *Browser) TransportName() string {
	if b == nil {
		return ""
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.transport == nil {
		return ""
	}
	return b.transport.Name()
}

// stubResponse is the canned response the stub Transport returns for
// every Fetch unless a custom hook is set via StubTransportOverride.
type stubResponse struct {
	status int
	header map[string]string
	body   []byte
}

// StubTransportOverride lets tests inject canned responses per URL into
// the default StubTransport. Thread-safe.
type StubTransportOverride struct {
	mu        sync.RWMutex
	byURL     map[string]stubResponse
	byDefault *stubResponse
}

// Set registers a canned response for a single URL.
func (o *StubTransportOverride) Set(url string, status int, body string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.byURL == nil {
		o.byURL = make(map[string]stubResponse)
	}
	o.byURL[url] = stubResponse{
		status: status,
		header: map[string]string{"Content-Type": "text/html"},
		body:   []byte(body),
	}
}

// SetDefault registers a canned response for any URL not in byURL.
func (o *StubTransportOverride) SetDefault(status int, body string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.byDefault = &stubResponse{
		status: status,
		header: map[string]string{"Content-Type": "text/html"},
		body:   []byte(body),
	}
}

// Lookup returns the canned response for the URL, or the default.
func (o *StubTransportOverride) Lookup(url string) (stubResponse, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if r, ok := o.byURL[url]; ok {
		return r, true
	}
	if o.byDefault != nil {
		return *o.byDefault, true
	}
	return stubResponse{}, false
}

// StubTransport is the default Transport. Returns deterministic stub
// output: Fetch returns a 200 + "stub body for <url>"; Render returns
// a tiny PNG-like header so callers can exercise the surface without
// spinning up Chromium.
type StubTransport struct {
	override *StubTransportOverride
}

// NewStubTransport returns a fresh StubTransport with no overrides.
func NewStubTransport() *StubTransport {
	return &StubTransport{override: &StubTransportOverride{}}
}

// Override returns the override registry for the stub transport so
// tests can push canned responses per URL.
func (s *StubTransport) Override() *StubTransportOverride {
	if s.override == nil {
		s.override = &StubTransportOverride{}
	}
	return s.override
}

// Name implements Transport.
func (s *StubTransport) Name() string { return "stub" }

// Fetch implements Transport. Returns canned response if registered,
// else a deterministic stub.
func (s *StubTransport) Fetch(_ context.Context, req Request) (Response, error) {
	if req.URL == "" {
		return Response{}, errors.New("browser: empty url")
	}
	start := time.Now()
	if s.override != nil {
		if r, ok := s.override.Lookup(req.URL); ok {
			return Response{
				StatusCode: r.status,
				Headers:    cloneHeader(r.header),
				Body:       append([]byte(nil), r.body...),
				Duration:   time.Since(start),
			}, nil
		}
	}
	body := fmt.Sprintf("stub body for %s\n", req.URL)
	if req.Body != nil && req.Method != http.MethodGet {
		body += fmt.Sprintf("(got %d body bytes)", len(req.Body))
	}
	return Response{
		StatusCode: 200,
		Headers:    map[string]string{"Content-Type": "text/html", "Server": "sin-stub/1.0"},
		Body:       []byte(body),
		Duration:   time.Since(start),
	}, nil
}

// Render implements Transport. Returns a tiny placeholder PNG header
// so callers that write the bytes to disk get something viewable.
func (s *StubTransport) Render(_ context.Context, url string, opts RenderOpts) (RenderResult, error) {
	if url == "" {
		return RenderResult{}, errors.New("browser: empty url")
	}
	start := time.Now()
	width, height := parseViewport(opts.Viewport)
	if opts.Format == "" {
		opts.Format = "png"
	}
	// 8-byte PNG signature followed by an IHDR stub so bytes.go file
	// inspection is byte-stable.
	header := pngStubHeader(width, height)
	body := fmt.Sprintf("<!-- stub render of %s -->\n", url)
	all := append(header[:], []byte(body)...)
	return RenderResult{
		Data:     all,
		Format:   opts.Format,
		Width:    width,
		Height:   height,
		URL:      url,
		Duration: time.Since(start),
	}, nil
}

// Close implements Transport. Stub has no resources.
func (s *StubTransport) Close() error { return nil }

// --- helpers ---------------------------------------------------------------

func cloneHeader(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func parseViewport(v string) (int, int) {
	if v == "" {
		return 1280, 720
	}
	var w, h int
	_, _ = fmt.Sscanf(v, "%dx%d", &w, &h)
	if w <= 0 || h <= 0 {
		return 1280, 720
	}
	return w, h
}

// pngStubHeader returns the 8-byte PNG signature followed by an IHDR
// chunk with the requested width/height. It is NOT a renderable PNG —
// just enough bytes for snapshot tests to assert shape.
func pngStubHeader(width, height int) [40]byte {
	var b [40]byte
	// PNG signature (8 bytes).
	copy(b[0:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	// IHDR chunk length (13) and "IHDR" tag.
	b[8] = 0x00
	b[9] = 0x00
	b[10] = 0x00
	b[11] = 0x0D
	copy(b[12:16], []byte{'I', 'H', 'D', 'R'})
	// Width (big-endian u32).
	b[16] = byte(width >> 24)
	b[17] = byte(width >> 16)
	b[18] = byte(width >> 8)
	b[19] = byte(width)
	// Height (big-endian u32).
	b[20] = byte(height >> 24)
	b[21] = byte(height >> 16)
	b[22] = byte(height >> 8)
	b[23] = byte(height)
	// Bit depth, color type, compression, filter, interlace (sane defaults).
	b[24] = 8
	b[25] = 2
	b[26] = 0
	b[27] = 0
	b[28] = 0
	// CRC stub — zero is fine for placeholder.
	b[29] = 0
	b[30] = 0
	b[31] = 0
	b[32] = 0
	// Pad with zeros.
	return b
}

// ioReader returns a bytes.Reader over the body so callers can pass
// req.Body without copying.
func ioReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}
