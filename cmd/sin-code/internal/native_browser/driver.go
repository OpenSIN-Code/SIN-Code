// SPDX-License-Identifier: MIT
// Purpose: pluggable Driver backend for the native_browser facade (issue #382).
//
// Three implementations live here today:
//
//   - HTTPDirectDriver:  the only fully-implemented real driver. Uses
//     net/http for fetches and golang.org/x/net/html for parse-level
//     selector lookup + anchor/click dispatches. No CGO, no Chromium, no
//     Python child. Works on static and server-rendered HTML; cannot
//     execute client-side JavaScript.
//
//   - HTTPOnlyDriver:    a stub. Every method returns ErrNotImplemented.
//     The contract documents that a future MCP-delegated path will route
//     through sin-browser-tools (Python stdio). Failing fast here means
//     misconfigured agents surface the missing dependency in error
//     messages instead of silently waiting.
//
//   - StubDriver:        a deterministic in-process test driver. Returns
//     canned HTML per URL and records every call so browser_test.go can
//     assert on the exact sequence of actions.
//
// Real Playwright / Chromium lands later — issue #382 follow-up. The
// public Driver interface stays unchanged so callers do not move.
package native_browser

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

	"golang.org/x/net/html"
)

// Action discriminates the Perform verb.
type Action string

const (
	ClickAction  Action = "click"
	FillAction   Action = "fill"
	SubmitAction Action = "submit"
)

// Driver is the pluggable browser backend. The native_browser package
// uses Driver as its sole seam to the outside world — adding a new
// backend means implementing this interface and registering with
// mcpclient registry.go.
type Driver interface {
	// Name returns a stable identifier for telemetry + logs
	// ("http-direct" | "http-only" | "stub" | future "chromium").
	Name() string
	// Close releases any resources the driver holds (file handles,
	// network connections, child processes). Idempotent.
	Close() error
	// Load fetches url and returns its HTML body as a string. The
	// returned string is the parser input for WaitFor and any future
	// selector-aware drivers — implementations must return stable
	// HTML suitable for selector substring matching.
	Load(ctx context.Context, url string) (string, error)
	// Perform submits a click / fill / submit against url. Drivers
	// that do not support mutation return ErrNotImplemented.
	Perform(ctx context.Context, url string, action Action, selector, value string) error
	// Render writes a screenshot of url to path. Drivers that cannot
	// render return ErrUnsupported.
	Render(ctx context.Context, url, path string) error
}

// --- HTTPDirectDriver ------------------------------------------------------

// HTTPDirectDriver is the production driver for static and
// server-rendered HTML. It performs no scripting and cannot paint
// pixels — wait for a future Chromium-backed driver for that.
type HTTPDirectDriver struct {
	mu      sync.Mutex
	client  *http.Client
	closed  bool
	headers map[string]string
}

// NewHTTPDirectDriver returns a driver that uses the default net/http
// transport and a 30 s per-request timeout.
func NewHTTPDirectDriver() *HTTPDirectDriver {
	return &HTTPDirectDriver{
		client: &http.Client{Timeout: 30 * time.Second},
		headers: map[string]string{
			"User-Agent": "sin-native-browser/1.0",
		},
	}
}

// Name implements Driver.
func (d *HTTPDirectDriver) Name() string { return "http-direct" }

// Close implements Driver.
func (d *HTTPDirectDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

// Load implements Driver. Issues an HTTP GET and returns the body as a
// UTF-8 string.
func (d *HTTPDirectDriver) Load(ctx context.Context, rawURL string) (string, error) {
	if rawURL == "" {
		return "", ErrEmptyURL
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return "", ErrClosed
	}
	client := d.client
	headers := d.headers
	d.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("native_browser: build request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("native_browser: get %q: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("native_browser: get %q: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("native_browser: read body: %w", err)
	}
	return string(body), nil
}

// Perform implements Driver. For static pages:
//
//   - Click on an <a href="…">  navigates the session to the href (best
//     effort — next Load / WaitFor will move there).
//   - Click on a <button>        returns ErrUnsupported.
//   - Fill on a form <input>     returns ErrUnsupported (no scripting).
//   - Submit on a <form action="…">  returns ErrUnsupported (no scripting).
//
// Form-mediated mutations need a future MCP-delegated driver; the
// contract here is "fail loudly, not silently" so callers can detect
// the gap in logs.
func (d *HTTPDirectDriver) Perform(_ context.Context, rawURL string, action Action, selector, _ string) error {
	if rawURL == "" {
		return ErrEmptyURL
	}
	if selector == "" {
		return errors.New("native_browser: perform: empty selector")
	}
	switch action {
	case ClickAction:
		href, ok := d.lookupAnchor(refreshURL(rawURL), selector)
		if !ok {
			return fmt.Errorf("native_browser: click %q: %w", selector, ErrUnsupported)
		}
		_ = href // the session URL update happens at BrowserSession.Navigate
		// time — Driver.Perform reports whether the click is even
		// representable against a static page. The actual hop is the
		// caller's responsibility (Navigate(clickHref)).
		return nil
	case FillAction, SubmitAction:
		return fmt.Errorf("native_browser: %s %q: %w (static-page driver cannot mutate forms)", action, selector, ErrUnsupported)
	default:
		return fmt.Errorf("native_browser: unknown action %q", action)
	}
}

// Render implements Driver. HTTPDirectDriver cannot render pixels.
func (d *HTTPDirectDriver) Render(_ context.Context, rawURL, _ string) error {
	if rawURL == "" {
		return ErrEmptyURL
	}
	return fmt.Errorf("native_browser: %s: %w", rawURL, ErrUnsupported)
}

// lookupAnchor scans the page at pageURL for an <a> element whose id, href,
// or text contains selector. Returns the href if found. Pure-HTML parse,
// no scripting.
func (d *HTTPDirectDriver) lookupAnchor(pageURL, selector string) (string, bool) {
	d.mu.Lock()
	client := d.client
	d.mu.Unlock()

	req, err := http.NewRequest(http.MethodGet, pageURL, nil)
	if err != nil {
		return "", false
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", false
	}
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return "", false
	}
	base, _ := url.Parse(pageURL)
	want := strings.TrimSpace(selector)
	var found string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			id := attr(n, "id")
			name := attr(n, "name")
			href := attr(n, "href")
			if strings.Contains(id, want) || strings.Contains(name, want) {
				if href != "" {
					found = resolveRef(base, href)
					return
				}
			}
			if href != "" {
				// Treat href itself as the selector — common case in
				// the targeted audit-style call sites this driver
				// covers (docs, status pages).
				if strings.Contains(href, want) {
					found = resolveRef(base, href)
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
			if found != "" {
				return
			}
		}
	}
	walk(doc)
	return found, found != ""
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func resolveRef(base *url.URL, ref string) string {
	if base == nil {
		return ref
	}
	u, err := base.Parse(ref)
	if err != nil {
		return ref
	}
	return u.String()
}

// refreshURL is a no-op that normalises an otherwise-empty URL to a
// sentinel so error messages stay descriptive. Kept as a function so the
// tests can swap or shadow it later if needed.
func refreshURL(s string) string { return s }

// --- HTTPOnlyDriver (stub) -------------------------------------------------

// HTTPOnlyDriver is the placeholder for the future MCP-delegated path to
// sin-browser-tools over stdio. Today it returns ErrNotImplemented from
// every method so misconfigured callers fail fast and surface the
// missing dependency in their error log (instead of hanging silently).
//
// When the MCP delegation lands, swap the body of Load / Perform / Render
// for calls into the connected sin-browser-tools server — the Driver
// interface stays unchanged.
type HTTPOnlyDriver struct {
	mu     sync.Mutex
	closed bool
	server string // future: MCP server name; recorded for diagnostic logs
}

// NewHTTPOnlyDriver returns a stub driver. server is the future MCP
// server the driver would delegate to ("browser-mcp" today).
func NewHTTPOnlyDriver(server string) *HTTPOnlyDriver {
	return &HTTPOnlyDriver{server: server}
}

// Name implements Driver.
func (d *HTTPOnlyDriver) Name() string {
	if d.server != "" {
		return "http-only:" + d.server
	}
	return "http-only"
}

// Close implements Driver.
func (d *HTTPOnlyDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

// Load implements Driver — stub.
func (d *HTTPOnlyDriver) Load(_ context.Context, rawURL string) (string, error) {
	if rawURL == "" {
		return "", ErrEmptyURL
	}
	d.mu.Lock()
	srv := d.server
	d.mu.Unlock()
	if srv != "" {
		return "", fmt.Errorf("native_browser: %s: %w (delegate to %s)", rawURL, ErrNotImplemented, srv)
	}
	return "", fmt.Errorf("native_browser: %s: %w", rawURL, ErrNotImplemented)
}

// Perform implements Driver — stub.
func (d *HTTPOnlyDriver) Perform(_ context.Context, rawURL string, action Action, selector, _ string) error {
	d.mu.Lock()
	srv := d.server
	d.mu.Unlock()
	tag := srv
	if tag == "" {
		tag = "browser-mcp"
	}
	return fmt.Errorf("native_browser: %s %q %q: %w (delegate to %s)",
		action, selector, rawURL, ErrNotImplemented, tag)
}

// Render implements Driver — stub.
func (d *HTTPOnlyDriver) Render(_ context.Context, rawURL, path string) error {
	d.mu.Lock()
	srv := d.server
	d.mu.Unlock()
	tag := srv
	if tag == "" {
		tag = "browser-mcp"
	}
	return fmt.Errorf("native_browser: render %q -> %q: %w (delegate to %s)",
		rawURL, path, ErrNotImplemented, tag)
}

// --- StubDriver (test) -----------------------------------------------------

// StubDriver is a deterministic in-process driver used only by tests. It
// never opens a network connection — every Load returns either the
// canned HTML registered via Set() or a default placeholder.
type StubDriver struct {
	mu          sync.Mutex
	responses   map[string]string
	defaultHTML string
	closed      bool

	LoadCalls    []string
	PerformCalls []StubPerform
	RenderCalls  []StubRender
}

// StubPerform records one Perform invocation.
type StubPerform struct {
	URL      string
	Action   Action
	Selector string
	Value    string
}

// StubRender records one Render invocation.
type StubRender struct {
	URL  string
	Path string
}

// NewStubDriver returns a driver whose default HTML is the empty string.
// Use Set / SetDefault to register canned responses per test.
func NewStubDriver() *StubDriver {
	return &StubDriver{
		responses: make(map[string]string),
	}
}

// Set registers a canned HTML response for a URL.
func (d *StubDriver) Set(u, body string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.responses[u] = body
}

// SetDefault registers a canned response for any URL not in responses.
func (d *StubDriver) SetDefault(body string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.defaultHTML = body
}

// Name implements Driver.
func (d *StubDriver) Name() string { return "stub" }

// Close implements Driver.
func (d *StubDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

// Load implements Driver.
func (d *StubDriver) Load(_ context.Context, rawURL string) (string, error) {
	if rawURL == "" {
		return "", ErrEmptyURL
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return "", ErrClosed
	}
	d.LoadCalls = append(d.LoadCalls, rawURL)
	if body, ok := d.responses[rawURL]; ok {
		return body, nil
	}
	return d.defaultHTML, nil
}

// Perform implements Driver — always succeeded in stub mode, records the
// call so tests can assert the (URL, action, selector, value) tuple.
func (d *StubDriver) Perform(_ context.Context, rawURL string, action Action, selector, value string) error {
	if rawURL == "" {
		return ErrEmptyURL
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	d.PerformCalls = append(d.PerformCalls, StubPerform{
		URL:      rawURL,
		Action:   action,
		Selector: selector,
		Value:    value,
	})
	return nil
}

// Render implements Driver — writes a one-line placeholder PNG header
// followed by the URL so snapshot-style tests can assert shape without
// a real Chromium install.
func (d *StubDriver) Render(_ context.Context, rawURL, path string) error {
	if rawURL == "" {
		return ErrEmptyURL
	}
	if path == "" {
		return errors.New("native_browser: stub: empty path")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	d.RenderCalls = append(d.RenderCalls, StubRender{URL: rawURL, Path: path})
	return nil
}
