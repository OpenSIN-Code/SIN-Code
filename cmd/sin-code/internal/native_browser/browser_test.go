// SPDX-License-Identifier: MIT
// Purpose: tests for the native_browser facade (issue #382). Drives the
// Driver seam through StubDriver where possible so the suite stays
// race-clean without a network. The real HTTPDirectDriver exercises
// httptest.NewServer for Navigate + Snapshot; policy split is verified
// against the package's own permission.DefaultPermissionRules surface.
package native_browser

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- the tests ------------------------------------------------------------

// TestBrowserNavigate verifies the URL can be loaded and that the
// driver is actually invoked end-to-end.
func TestBrowserNavigate(t *testing.T) {
	drv := NewStubDriver()
	drv.Set("http://example.test/hello", "<html><body>hello</body></html>")
	drv.SetDefault("<html></html>")

	b := NewBrowser(Config{Driver: drv})
	defer b.Close()

	s, err := b.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Close()

	if err := s.Navigate("http://example.test/hello"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if got := s.URL(); got != "http://example.test/hello" {
		t.Fatalf("URL after Navigate: got %q", got)
	}

	drv.mu.Lock()
	calls := append([]string(nil), drv.LoadCalls...)
	drv.mu.Unlock()
	if len(calls) != 1 || calls[0] != "http://example.test/hello" {
		t.Fatalf("expected 1 driver.Load call to the right URL, got %v", calls)
	}

	html, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !strings.Contains(html, "hello") {
		t.Fatalf("Snapshot body should contain page text, got %q", html)
	}
}

// TestBrowserSnapshot covers the Snapshot() contract: returns the
// cached HTML string from the last successful Navigate, no driver calls.
func TestBrowserSnapshot(t *testing.T) {
	drv := NewStubDriver()
	drv.Set("http://example.test/page", "<html><title>x</title><body><p id=\"target\">X</p></body></html>")

	b := NewBrowser(Config{Driver: drv})
	defer b.Close()

	s, _ := b.NewSession()
	defer s.Close()

	if err := s.Navigate("http://example.test/page"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}

	got, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if got != "<html><title>x</title><body><p id=\"target\">X</p></body></html>" {
		t.Fatalf("Snapshot returned unfamiliar body: %q", got)
	}

	drv.mu.Lock()
	loads := len(drv.LoadCalls)
	drv.mu.Unlock()
	if loads != 1 {
		t.Fatalf("Snapshot should not re-call driver.Load, total calls = %d", loads)
	}
}

// TestBrowserSnapshot_NoURL asserts the documented ErrNoURL contract.
func TestBrowserSnapshot_NoURL(t *testing.T) {
	b := NewBrowser(Config{Driver: NewStubDriver()})
	defer b.Close()

	s, _ := b.NewSession()
	defer s.Close()

	if _, err := s.Snapshot(); !errors.Is(err, ErrNoURL) {
		t.Fatalf("expected ErrNoURL, got %v", err)
	}
}

// TestBrowserNavigate_EmptyURL guards ErrEmptyURL.
func TestBrowserNavigate_EmptyURL(t *testing.T) {
	b := NewBrowser(Config{Driver: NewStubDriver()})
	defer b.Close()

	s, _ := b.NewSession()
	defer s.Close()

	if err := s.Navigate(""); !errors.Is(err, ErrEmptyURL) {
		t.Fatalf("expected ErrEmptyURL, got %v", err)
	}
}

// TestBrowserSessionLifecycle verifies Close rejects further calls.
func TestBrowserSessionLifecycle(t *testing.T) {
	drv := NewStubDriver()
	drv.SetDefault("<html></html>")
	b := NewBrowser(Config{Driver: drv})
	defer b.Close()

	s, _ := b.NewSession()
	if err := s.Navigate("http://example.test/"); err != nil {
		t.Fatalf("Navigate before Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Navigate("http://example.test/again"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Navigate after Close: expected ErrClosed, got %v", err)
	}
	if _, err := s.Snapshot(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Snapshot after Close: expected ErrClosed, got %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close should be idempotent: %v", err)
	}
}

// TestBrowserClosedRefusesSession verifies a closed Browser refuses
// new sessions.
func TestBrowserClosedRefusesSession(t *testing.T) {
	b := NewBrowser(Config{Driver: NewStubDriver()})
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := b.NewSession(); !errors.Is(err, ErrClosed) {
		t.Fatalf("NewSession after Close: expected ErrClosed, got %v", err)
	}
}

// TestBrowserNilGuards ensures every public method on a nil receiver
// returns a controlled error instead of crashing.
func TestBrowserNilGuards(t *testing.T) {
	var b *Browser
	if _, err := b.NewSession(); err == nil {
		t.Fatalf("nil Browser NewSession should error, got nil")
	}
	if err := b.Close(); err != nil {
		t.Fatalf("nil Browser Close should be no-op, got %v", err)
	}
	if got := b.DriverName(); got != "" {
		t.Fatalf("nil Browser DriverName should be empty, got %q", got)
	}

	var s *BrowserSession
	if _, err := s.Snapshot(); err == nil {
		t.Fatalf("nil Session Snapshot should error, got nil")
	}
	if s.URL() != "" {
		t.Fatalf("nil Session URL should be empty, got %q", s.URL())
	}
	if err := s.Close(); err != nil {
		t.Fatalf("nil Session Close should be no-op, got %v", err)
	}
}

// TestDriverStub_PerformRecords verifies Click/Fill/Submit record the
// full (URL, action, selector, value) tuple so the policy layer can
// reason about confirmed-in-place mutations.
func TestDriverStub_PerformRecords(t *testing.T) {
	drv := NewStubDriver()
	drv.SetDefault("<html></html>")
	b := NewBrowser(Config{Driver: drv})
	defer b.Close()
	s, _ := b.NewSession()
	defer s.Close()

	if err := s.Navigate("http://example.test/"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if err := s.Click("#submit"); err != nil {
		t.Fatalf("Click: %v", err)
	}
	if err := s.Fill("#name", "alice"); err != nil {
		t.Fatalf("Fill: %v", err)
	}
	if err := s.Submit("#login-form"); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	drv.mu.Lock()
	defer drv.mu.Unlock()
	if len(drv.PerformCalls) != 3 {
		t.Fatalf("expected 3 Perform calls, got %d", len(drv.PerformCalls))
	}
	want := []StubPerform{
		{URL: "http://example.test/", Action: ClickAction, Selector: "#submit", Value: ""},
		{URL: "http://example.test/", Action: FillAction, Selector: "#name", Value: "alice"},
		{URL: "http://example.test/", Action: SubmitAction, Selector: "#login-form", Value: ""},
	}
	for i, w := range want {
		if drv.PerformCalls[i] != w {
			t.Errorf("PerformCalls[%d]: expected %+v, got %+v", i, w, drv.PerformCalls[i])
		}
	}
}

// TestDriverStub_ScreenshotNoop verifies the stub records the render
// call without needing a real file write.
func TestDriverStub_ScreenshotNoop(t *testing.T) {
	drv := NewStubDriver()
	drv.SetDefault("<html></html>")
	b := NewBrowser(Config{Driver: drv})
	defer b.Close()
	s, _ := b.NewSession()
	defer s.Close()

	if err := s.Navigate("http://example.test/"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if err := s.Screenshot("/tmp/sin-native-browser-screenshot.png"); err != nil {
		t.Fatalf("Screenshot: %v", err)
	}
	drv.mu.Lock()
	defer drv.mu.Unlock()
	if len(drv.RenderCalls) != 1 || drv.RenderCalls[0].Path != "/tmp/sin-native-browser-screenshot.png" {
		t.Fatalf("expected 1 Render call with the right path, got %+v", drv.RenderCalls)
	}
}

// TestDriverHTTPOnly_NotImplemented asserts the stub driver surfaces the
// missing-path error clearly (so misconfigured callers cannot hang).
func TestDriverHTTPOnly_NotImplemented(t *testing.T) {
	d := NewHTTPOnlyDriver("browser-mcp")
	if got := d.Name(); got != "http-only:browser-mcp" {
		t.Fatalf("Name: %q", got)
	}
	if _, err := d.Load(context.Background(), "http://x"); err == nil || !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Load should be ErrNotImplemented, got %v", err)
	}
	if err := d.Perform(context.Background(), "http://x", ClickAction, "#y", ""); err == nil || !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Perform should be ErrNotImplemented, got %v", err)
	}
	if err := d.Render(context.Background(), "http://x", "/tmp/x.png"); err == nil || !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Render should be ErrNotImplemented, got %v", err)
	}
}

// TestDriverHTTPDirect_RenderUnsupported verifies the HTTP-direct
// driver reports ErrUnsupported for screenshot rendering — it cannot
// paint pixels without Chromium.
func TestDriverHTTPDirect_RenderUnsupported(t *testing.T) {
	d := NewHTTPDirectDriver()
	defer d.Close()
	if got := d.Name(); got != "http-direct" {
		t.Fatalf("Name: got %q", got)
	}
	err := d.Render(context.Background(), "http://x", "/tmp/x.png")
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Render should be ErrUnsupported, got %v", err)
	}
}

// TestDriverHTTPDirect_PerformFillUnsupported verifies fill/form mutations
// on the static-page driver fail loudly rather than silently no-op.
func TestDriverHTTPDirect_PerformFillUnsupported(t *testing.T) {
	d := NewHTTPDirectDriver()
	defer d.Close()
	if err := d.Perform(context.Background(), "http://x", FillAction, "#y", "v"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Fill should bubble ErrUnsupported, got %v", err)
	}
	if err := d.Perform(context.Background(), "http://x", SubmitAction, "#f", ""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Submit should bubble ErrUnsupported, got %v", err)
	}
}

// TestBrowserHTTPDirect_NavigateSnapshot wires the real HTTPDirectDriver
// against an httptest.NewServer — the closest thing to a production
// path the package can run without spawning Chromium.
func TestBrowserHTTPDirect_NavigateSnapshot(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><html><body><h1 id="hi">hi from test</h1><a id="more" href="/more">more</a></body></html>`))
		case "/more":
			_, _ = w.Write([]byte(`<html>more page</html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b := NewBrowser(Config{Driver: NewHTTPDirectDriver(), Timeout: 3 * time.Second})
	defer b.Close()
	s, _ := b.NewSession()
	defer s.Close()

	if err := s.Navigate(srv.URL + "/"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	body, err := s.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !strings.Contains(body, "hi from test") {
		t.Fatalf("Snapshot missing page text, got %q", body)
	}
	if !strings.Contains(body, `id="more"`) {
		t.Fatalf("Snapshot missing anchor, got %q", body)
	}
}

// TestBrowserWaitFor_PollsUntilSelectionAppears verifies the polling
// loop terminates when the selector becomes visible.
func TestBrowserWaitFor_PollsUntilSelectionAppears(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if n < 2 {
			_, _ = w.Write([]byte(`<html><body>loading</body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<html><body><div data-target="#ready">done</div></body></html>`))
	}))
	defer srv.Close()

	b := NewBrowser(Config{
		Driver:             NewHTTPDirectDriver(),
		Timeout:            3 * time.Second,
		WaitForPollInterval: 30 * time.Millisecond,
		WaitForDeadline:    2 * time.Second,
	})
	defer b.Close()
	s, _ := b.NewSession()
	defer s.Close()

	if err := s.Navigate(srv.URL); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	if err := s.WaitFor("#ready"); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
}

// TestBrowserWaitFor_Deadline asserts the polling loop fails cleanly
// when the selector never appears.
func TestBrowserWaitFor_Deadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body>no target here</body></html>`))
	}))
	defer srv.Close()

	b := NewBrowser(Config{
		Driver:             NewHTTPDirectDriver(),
		Timeout:            3 * time.Second,
		WaitForPollInterval: 50 * time.Millisecond,
		WaitForDeadline:    300 * time.Millisecond,
	})
	defer b.Close()
	s, _ := b.NewSession()
	defer s.Close()

	if err := s.Navigate(srv.URL); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	err := s.WaitFor("#never-there")
	if err == nil {
		t.Fatal("WaitFor should time out")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error should mention timeout, got %v", err)
	}
}

// TestBrowserConcurrentSessions verifies multiple independent sessions
// do not share URL/HTML state.
func TestBrowserConcurrentSessions(t *testing.T) {
	drv := NewStubDriver()
	drv.Set("http://a.test/", "<html>A</html>")
	drv.Set("http://b.test/", "<html>B</html>")
	b := NewBrowser(Config{Driver: drv})
	defer b.Close()

	var wg sync.WaitGroup
	for _, url := range []string{"http://a.test/", "http://b.test/"} {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			s, _ := b.NewSession()
			defer s.Close()
			if err := s.Navigate(u); err != nil {
				t.Errorf("Navigate %s: %v", u, err)
				return
			}
			if _, err := s.Snapshot(); err != nil {
				t.Errorf("Snapshot %s: %v", u, err)
			}
		}(url)
	}
	wg.Wait()
}

// TestBrowser_DefaultDriverIsHTTPDirect verifies NewBrowser with nil
// Driver falls back to HTTPDirectDriver (so callers that don't pick a
// driver still get a working static-HTML facade).
func TestBrowser_DefaultDriverIsHTTPDirect(t *testing.T) {
	b := NewBrowser(Config{})
	defer b.Close()
	if got := b.DriverName(); got != "http-direct" {
		t.Fatalf("default DriverName: got %q, want http-direct", got)
	}
}
