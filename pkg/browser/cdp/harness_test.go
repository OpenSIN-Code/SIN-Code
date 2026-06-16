// SPDX-License-Identifier: MIT
// Purpose: browser integration test harness for pkg/browser/cdp.
//
// These tests require Chrome or Chromium. They skip automatically when no
// browser is found so CI stays green without a browser while still running
// the deterministic unit tests in analyze_test.go.
//
// Run browser tests explicitly:
//
//	go test ./pkg/browser/cdp/...
//
// Or with a non-standard Chrome path:
//
//	CHROME_PATH=/opt/chrome/chrome go test ./pkg/browser/cdp/...
package cdp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// runScenario launches headless Chrome, navigates to url, waits settle, and
// returns the BuildReport result. Each test runs in its own temp dir.
func runScenario(t *testing.T, url string, settle time.Duration) *Report {
	t.Helper()

	dir := t.TempDir()
	cfg := DefaultConfig(filepath.Join(dir, "evidence.jsonl"))
	cfg.MetricsEvery = 0 // determinism: no background polling in tests
	rec, err := NewRecorder(cfg)
	if err != nil {
		t.Fatalf("recorder: %v", err)
	}
	defer rec.Close()

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
		)...)
	defer cancelAlloc()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	rec.Attach(ctx)
	if err := rec.EnableDomains(ctx); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := rec.InstallVitals(ctx); err != nil {
		t.Fatalf("vitals: %v", err)
	}

	rec.SetStep("navigate")
	if err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.Sleep(settle),
	); err != nil {
		t.Logf("nav (non-fatal): %v", err)
	}
	rec.EvalVitalsNow(ctx)

	return BuildReport(rec.Events(), 25)
}

// hasFinding reports whether the report contains a Finding whose Signature
// starts with sigPrefix.
func hasFinding(r *Report, sigPrefix string) bool {
	for _, f := range r.Findings {
		if strings.HasPrefix(f.Signature, sigPrefix) {
			return true
		}
	}
	return false
}

// hasFixClass reports whether the report contains a Suggestion with the given
// FixClass value.
func hasFixClass(r *Report, fixClass string) bool {
	for _, s := range r.Suggestions {
		if s.FixClass == fixClass {
			return true
		}
	}
	return false
}

// skipIfNoChrome skips the test when no Chrome/Chromium binary is found.
func skipIfNoChrome(t *testing.T) {
	t.Helper()
	for _, p := range []string{
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	} {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	if os.Getenv("CHROME_PATH") == "" {
		t.Skip("no Chrome/Chromium available; set CHROME_PATH to run browser harness")
	}
}

func TestConsoleAndException(t *testing.T) {
	skipIfNoChrome(t)
	srv := NewFaultServer()
	defer srv.Close()

	r := runScenario(t, srv.URL+"/console-error", time.Second)
	if !hasFinding(r, "console:") {
		t.Error("expected a console error finding")
	}
	if !hasFinding(r, "exc:") {
		t.Error("expected an uncaught exception finding")
	}
	if !r.Summary.HasFatal {
		t.Error("expected HasFatal=true for a page with errors")
	}
}

func TestHTTP500(t *testing.T) {
	skipIfNoChrome(t)
	srv := NewFaultServer()
	defer srv.Close()

	r := runScenario(t, srv.URL+"/http-500", time.Second)
	if !hasFinding(r, "http:500") {
		t.Error("expected an http:500 finding")
	}
	if !hasFixClass(r, "network.http_error") {
		t.Error("expected a network.http_error suggestion")
	}
}

func TestNetworkFailure(t *testing.T) {
	skipIfNoChrome(t)
	srv := NewFaultServer()
	defer srv.Close()

	r := runScenario(t, srv.URL+"/net-fail", 2*time.Second)
	if !hasFinding(r, "netfail:") && !hasFinding(r, "netblock:") {
		t.Error("expected a network failure finding")
	}
}

func TestCSPViolation(t *testing.T) {
	skipIfNoChrome(t)
	srv := NewFaultServer()
	defer srv.Close()

	// Settle long enough for the Audits issueAdded event to arrive.
	r := runScenario(t, srv.URL+"/csp", 1500*time.Millisecond)
	if !hasFinding(r, "audit:") {
		t.Error("expected an Audits issue finding for CSP violation")
	}
}

func TestSlowLCP(t *testing.T) {
	skipIfNoChrome(t)
	srv := NewFaultServer()
	defer srv.Close()

	// Settle long enough for the delayed hero image to become the LCP element
	// and for the PerformanceObserver to fire.
	r := runScenario(t, srv.URL+"/slow-lcp", 2500*time.Millisecond)
	if !hasFinding(r, "vital:LCP") {
		t.Error("expected an LCP vital finding for the slow hero image")
	}
}

func TestCleanPageHasNoErrors(t *testing.T) {
	skipIfNoChrome(t)
	srv := NewFaultServer()
	defer srv.Close()

	r := runScenario(t, srv.URL+"/clean", time.Second)
	if r.Summary.HasFatal {
		t.Errorf("clean page should not be fatal; findings=%+v", r.Findings)
	}
	if r.Summary.Errors != 0 {
		t.Errorf("clean page should have 0 errors, got %d", r.Summary.Errors)
	}
}

// TestJSONLGroundTruthWritten verifies the on-disk log is non-empty after
// a navigation and that Close() flushes the sink correctly.
func TestJSONLGroundTruthWritten(t *testing.T) {
	skipIfNoChrome(t)
	srv := NewFaultServer()
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "gt.jsonl")
	cfg := DefaultConfig(path)
	cfg.MetricsEvery = 0
	rec, _ := NewRecorder(cfg)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("no-sandbox", true),
		)...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	rec.Attach(ctx)
	rec.EnableDomains(ctx)
	chromedp.Run(ctx,
		chromedp.Navigate(srv.URL+"/console-error"),
		chromedp.Sleep(time.Second),
	)
	rec.Close() // flush sink before stat

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("jsonl not written: %v", err)
	}
	if info.Size() == 0 {
		t.Error("jsonl ground truth is empty")
	}
}
