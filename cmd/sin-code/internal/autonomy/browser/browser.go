// SPDX-License-Identifier: MIT
// Purpose: headless browser-automation primitives for the autonomy layer
// (issue #382). Provides a Transport-based HTTP fetcher and a thin
// Navigate/Render surface that future headless-browser backends (Chromium
// CDP, Playwright) can implement. The default transport is the Go stdlib
// HTTP client, keeping the package CGO-free (mandate M2).
package browser

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/egress"
)

// Request is a single HTTP fetch specification.
type Request struct {
	URL     string
	Method  string
	Headers map[string]string
	Body    []byte
	Timeout time.Duration
}

// Response is the result of a Fetch call.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       []byte
	Duration   time.Duration
}

// Transport is the interface backends implement. The default stdlib
// implementation lives in this file; tests inject StubTransport.
type Transport interface {
	Fetch(ctx context.Context, req Request) (Response, error)
}

// RenderOpts configures a Render call (stub for future headless rendering).
type RenderOpts struct {
	Width      int
	Height     int
	WaitFor    string // CSS selector to wait for
	Timeout    time.Duration
	JavaScript string // optional script to run before capture
	UserAgent  string
}

// RenderResult is the output of a Render call (stub for future rendering).
type RenderResult struct {
	HTML       string
	Screenshot []byte
	Title      string
	URL        string
	Duration   time.Duration
}

// Page represents a navigated URL with its fetched content.
type Page struct {
	URL      string
	Response Response
}

// URLPolicy controls which network destinations the autonomy browser may reach.
// The secure default blocks localhost, private, link-local, multicast, and
// unspecified addresses. Tests and explicitly trusted local workflows can opt in.
type URLPolicy = egress.Policy

// Browser wraps a Transport and provides Navigate/Render helpers.
type Browser struct {
	transport Transport
	policy    URLPolicy
	mu        sync.Mutex
	defaultUA string
}

// NewBrowser returns a Browser with the secure default URL policy.
func NewBrowser(t Transport) *Browser {
	return NewBrowserWithPolicy(t, URLPolicy{})
}

// NewBrowserWithPolicy returns a Browser backed by t and the supplied policy.
// A nil transport uses the guarded stdlib HTTP transport.
func NewBrowserWithPolicy(t Transport, policy URLPolicy) *Browser {
	if t == nil {
		t = &stdlibTransport{policy: policy}
	}
	return &Browser{transport: t, policy: policy, defaultUA: "SIN-Code-Browser/1.0"}
}

// Fetch issues an HTTP request through the transport and returns the
// response. A timeout of 0 falls back to the context deadline.
func (b *Browser) Fetch(ctx context.Context, req Request) (Response, error) {
	if _, err := egress.ValidateURL(req.URL, b.policy); err != nil {
		return Response{}, fmt.Errorf("browser: %w", err)
	}
	if req.Method == "" {
		req.Method = http.MethodGet
	}
	if req.Headers == nil {
		req.Headers = map[string]string{}
	}
	if _, ok := req.Headers["User-Agent"]; !ok {
		req.Headers["User-Agent"] = b.defaultUA
	}
	return b.transport.Fetch(ctx, req)
}

// Navigate fetches a URL and wraps the response in a Page.
func (b *Browser) Navigate(ctx context.Context, url string) (*Page, error) {
	resp, err := b.Fetch(ctx, Request{URL: url, Method: http.MethodGet})
	if err != nil {
		return nil, fmt.Errorf("navigate %s: %w", url, err)
	}
	return &Page{URL: url, Response: resp}, nil
}

// Render is a stub that fetches the page HTML via the transport. A real
// headless backend (Chromium CDP) would replace this to produce
// screenshots and execute JavaScript. The stub returns the raw HTML so
// callers can verify the surface without a browser dependency.
func (b *Browser) Render(ctx context.Context, url string, opts RenderOpts) (RenderResult, error) {
	start := time.Now()
	if opts.UserAgent == "" {
		opts.UserAgent = b.defaultUA
	}
	req := Request{
		URL:    url,
		Method: http.MethodGet,
		Headers: map[string]string{
			"User-Agent": opts.UserAgent,
		},
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	resp, err := b.Fetch(ctx, req)
	if err != nil {
		return RenderResult{}, fmt.Errorf("render %s: %w", url, err)
	}
	return RenderResult{
		HTML:     string(resp.Body),
		URL:      url,
		Duration: time.Since(start),
	}, nil
}

// --- stdlib transport ------------------------------------------------------

// stdlibTransport is the default Transport backed by net/http.
type stdlibTransport struct {
	policy URLPolicy
}

func (s *stdlibTransport) Fetch(ctx context.Context, req Request) (Response, error) {
	if err := egress.Check(ctx, req.URL, s.policy); err != nil {
		return Response{}, err
	}
	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = newBytesReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	if err != nil {
		return Response{}, err
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	transport := &http.Transport{
		Proxy:       nil,
		DialContext: guardedDialContext(s.policy),
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(next *http.Request, _ []*http.Request) error {
			return egress.Check(next.Context(), next.URL.String(), s.policy)
		},
	}
	if req.Timeout > 0 {
		client.Timeout = req.Timeout
	}
	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}
	headers := make(map[string]string, len(resp.Header))
	for k, vs := range resp.Header {
		if len(vs) > 0 {
			headers[k] = vs[0]
		}
	}
	return Response{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
		Duration:   time.Since(start),
	}, nil
}

func guardedDialContext(policy URLPolicy) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if policy.AllowPrivateNetworks {
			return dialer.DialContext(ctx, network, address)
		}
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid destination %q: %w", address, err)
		}
		resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", host, err)
		}
		var lastErr error
		for _, candidate := range resolved {
			if egress.IsPrivate(candidate.IP) {
				continue
			}
			conn, dialErr := dialer.DialContext(
				ctx, network, net.JoinHostPort(candidate.IP.String(), port),
			)
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("all resolved addresses for %q are blocked", host)
	}
}

// bytesReader wraps a []byte as an io.Reader without importing bytes at
// the package level (kept inline to minimise the dependency footprint).
type bytesReader struct {
	data []byte
	off  int
}

func newBytesReader(b []byte) *bytesReader { return &bytesReader{data: b} }

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

// --- stub transport (for tests) --------------------------------------------

// StubTransport is a test Transport that returns a preconfigured Response
// and records the last Request it received.
type StubTransport struct {
	Response Response
	Err      error
	LastReq  Request
}

func (s *StubTransport) Fetch(ctx context.Context, req Request) (Response, error) {
	s.LastReq = req
	if s.Err != nil {
		return Response{}, s.Err
	}
	return s.Response, nil
}
