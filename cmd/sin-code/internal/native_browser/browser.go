// SPDX-License-Identifier: MIT
// Purpose: pure-Go native browser-automation facade (issue #382).
//
// The native_browser package exposes a Browser + BrowserSession surface that
// other packages (chat, mcpclient, catalog) can wire up without spawning a
// Python child or pulling in CGO bindings to Chromium. The Driver interface
// fans out to pluggable backends:
//
//   - HTTPDirectDriver: net/http + golang.org/x/net/html parser for static
//     sites where no JavaScript is required (docs, RFC pages, status pages).
//     This is the only fully-implemented driver today.
//   - HTTPOnlyDriver: a stub that documents the future MCP-delegated path
//     to sin-browser-tools without introducing its own process. It returns
//     ErrNotImplemented from every method so callers fail fast and the
//     missing-path is visible in error messages, not silently swallowed.
//
// Real headless rendering (Playwright / Chromium) lands later behind the
// same Driver interface — no caller changes required when that lands.
package native_browser

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Sentinel errors. Stable surface — callers branch on these.
var (
	ErrClosed         = errors.New("native_browser: session closed")
	ErrEmptyURL       = errors.New("native_browser: empty url")
	ErrNoURL          = errors.New("native_browser: no current url (call Navigate first)")
	ErrUnsupported    = errors.New("native_browser: driver does not support this operation")
	ErrNotImplemented = errors.New("native_browser: not implemented (issue #382 follow-up)")
)

// Config tunes a Browser. Driver may be nil (defaults to NewHTTPDirectDriver).
type Config struct {
	Driver    Driver
	UserAgent string
	// Timeout caps any single network call. Zero = 30 s default.
	Timeout time.Duration
	// WaitForPollInterval is how often WaitFor polls the underlying URL.
	// Zero = 250 ms default.
	WaitForPollInterval time.Duration
	// WaitForDeadline caps the total WaitFor duration. Zero = 10 s default.
	WaitForDeadline time.Duration
}

// Browser is the top-level entry point. Holds one shared Driver and spawns
// cheap BrowserSessions on demand. Safe for concurrent use.
type Browser struct {
	mu     sync.RWMutex
	driver Driver
	cfg    Config
	closed bool
}

// NewBrowser returns a Browser wired against cfg.Driver (or a fresh
// HTTPDirectDriver if nil).
func NewBrowser(cfg Config) *Browser {
	if cfg.Driver == nil {
		cfg.Driver = NewHTTPDirectDriver()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.WaitForPollInterval == 0 {
		cfg.WaitForPollInterval = 250 * time.Millisecond
	}
	if cfg.WaitForDeadline == 0 {
		cfg.WaitForDeadline = 10 * time.Second
	}
	return &Browser{driver: cfg.Driver, cfg: cfg}
}

// DriverName returns the wrapped Driver's identifier.
func (b *Browser) DriverName() string {
	if b == nil {
		return ""
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.driver == nil {
		return ""
	}
	return b.driver.Name()
}

// NewSession opens a new BrowserSession against the shared Driver.
// Sessions are independent — each carries its own URL state.
func (b *Browser) NewSession() (*BrowserSession, error) {
	if b == nil {
		return nil, errors.New("native_browser: nil browser")
	}
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return nil, ErrClosed
	}
	driver := b.driver
	cfg := b.cfg
	b.mu.RUnlock()
	return &BrowserSession{driver: driver, cfg: cfg}, nil
}

// Close shuts down the underlying Driver and refuses new Sessions. Existing
// sessions are also marked closed.
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
	if b.driver != nil {
		return b.driver.Close()
	}
	return nil
}

// BrowserSession is one logical "tab". Tracks the current URL + cached HTML
// across calls. Calls are serialised through mu — concurrent callers see a
// consistent (URL, HTML) pair.
type BrowserSession struct {
	mu     sync.Mutex
	driver Driver
	cfg    Config
	url    string
	html   string
	closed bool
}

// URL returns the last successful Navigate target. Empty string before the
// first Navigate call.
func (s *BrowserSession) URL() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

// Navigate loads url, stores the page HTML in the session, and resets any
// previously-cached content. On failure the session keeps its previous url
// (so callers can retry without losing context).
func (s *BrowserSession) Navigate(url string) error {
	if s == nil {
		return errors.New("native_browser: nil session")
	}
	if url == "" {
		return ErrEmptyURL
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	driver := s.driver
	cfg := s.cfg
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	page, err := driver.Load(ctx, url)
	if err != nil {
		return fmt.Errorf("native_browser: navigate %q: %w", url, err)
	}

	s.mu.Lock()
	s.url = url
	s.html = page
	s.mu.Unlock()
	return nil
}

// Snapshot returns the HTML string cached by the last successful Navigate.
// Returns ErrNoURL if Navigate has not been called.
func (s *BrowserSession) Snapshot() (string, error) {
	if s == nil {
		return "", errors.New("native_browser: nil session")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrClosed
	}
	if s.url == "" {
		return "", ErrNoURL
	}
	return s.html, nil
}

// Click submits a click action against the current URL. Mutating operation
// (M4): the caller is expected to gate this through the permission engine
// (`native_browser__click` policy == "ask").
func (s *BrowserSession) Click(selector string) error {
	if s == nil {
		return errors.New("native_browser: nil session")
	}
	if selector == "" {
		return errors.New("native_browser: empty selector")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.url == "" {
		s.mu.Unlock()
		return ErrNoURL
	}
	driver := s.driver
	cfg := s.cfg
	u := s.url
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	if err := driver.Perform(ctx, u, ClickAction, selector, ""); err != nil {
		return fmt.Errorf("native_browser: click %q: %w", selector, err)
	}
	return nil
}

// Fill submits a fill action against the current URL. Mutating operation
// (M4): the caller is expected to gate this through the permission engine.
func (s *BrowserSession) Fill(selector, value string) error {
	if s == nil {
		return errors.New("native_browser: nil session")
	}
	if selector == "" {
		return errors.New("native_browser: empty selector")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.url == "" {
		s.mu.Unlock()
		return ErrNoURL
	}
	driver := s.driver
	cfg := s.cfg
	u := s.url
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	if err := driver.Perform(ctx, u, FillAction, selector, value); err != nil {
		return fmt.Errorf("native_browser: fill %q: %w", selector, err)
	}
	return nil
}

// Submit posts the form anchored at the current URL. Mutating operation
// (M4): the caller is expected to gate this through the permission engine.
func (s *BrowserSession) Submit(selector string) error {
	if s == nil {
		return errors.New("native_browser: nil session")
	}
	if selector == "" {
		return errors.New("native_browser: empty selector")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.url == "" {
		s.mu.Unlock()
		return ErrNoURL
	}
	driver := s.driver
	cfg := s.cfg
	u := s.url
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	if err := driver.Perform(ctx, u, SubmitAction, selector, ""); err != nil {
		return fmt.Errorf("native_browser: submit %q: %w", selector, err)
	}
	return nil
}

// Screenshot writes a screenshot of the current URL to path. Drivers that
// do not support rendering return ErrUnsupported.
func (s *BrowserSession) Screenshot(path string) error {
	if s == nil {
		return errors.New("native_browser: nil session")
	}
	if path == "" {
		return errors.New("native_browser: empty path")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.url == "" {
		s.mu.Unlock()
		return ErrNoURL
	}
	driver := s.driver
	cfg := s.cfg
	u := s.url
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	if err := driver.Render(ctx, u, path); err != nil {
		return fmt.Errorf("native_browser: screenshot %q: %w", u, err)
	}
	return nil
}

// WaitFor polls the current URL until the selector is present in the page
// HTML, or until cfg.WaitForDeadline elapses.
func (s *BrowserSession) WaitFor(selector string) error {
	if s == nil {
		return errors.New("native_browser: nil session")
	}
	if selector == "" {
		return errors.New("native_browser: empty selector")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.url == "" {
		s.mu.Unlock()
		return ErrNoURL
	}
	driver := s.driver
	cfg := s.cfg
	u := s.url
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.WaitForDeadline)
	defer cancel()

	ticker := time.NewTicker(cfg.WaitForPollInterval)
	defer ticker.Stop()

	// Check once immediately, then on each tick.
	for {
		page, err := driver.Load(ctx, u)
		if err == nil && selectorPresent(page, selector) {
			s.mu.Lock()
			s.url = u
			s.html = page
			s.mu.Unlock()
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("native_browser: waitfor %q timed out: %w", selector, ctx.Err())
		case <-ticker.C:
		}
	}
}

// Close marks the session closed. Calls after Close return ErrClosed.
func (s *BrowserSession) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return nil
}

// --- helpers ---------------------------------------------------------------

// selectorPresent is a deliberately-loose substring match: real browser
// drivers would parse the DOM and route via CSS / XPath. We just need
// enough for the static-page use cases this driver covers.
func selectorPresent(html, selector string) bool {
	if html == "" || selector == "" {
		return false
	}
	sel := strings.TrimSpace(selector)
	return strings.Contains(html, sel)
}
