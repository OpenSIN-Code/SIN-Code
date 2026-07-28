// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when tools are MCP-externalized
// Purpose: Browser CDP tool implementations — sin_browser_navigate,
// sin_browser_findings, sin_browser_snapshot, sin_browser_vitals_flush,
// sin_browser_diff. These drive a headless Chrome instance via the
// Chrome DevTools Protocol to capture the full event stream (network,
// console, exceptions, DevTools Audits, security, Web Vitals) and
// surface deterministic Findings. Specs and dispatch remain in
// chat_tools_extra.go.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/egress"
	"github.com/OpenSIN-Code/SIN-Code/pkg/browser/cdp"
)

// ----------------------------------------------------------------------------
// Browser CDP session state — process-wide singleton for the agent loop.
// Only one recording session is active at a time; sin_browser_navigate
// replaces any previous session.
// ----------------------------------------------------------------------------

var (
	browserSession *activeBrowserSession
)

type activeBrowserSession struct {
	rec       *cdp.Recorder
	cancelCtx context.CancelFunc
	jsonlPath string
	// cdpCtx is kept alive so EvalVitalsNow can run after navigation completes.
	cdpCtx context.Context
	// baseline is the Report saved from a prior run for DiffReports comparison.
	baseline *cdp.Report
}

// toolBrowserNavigate drives headless Chrome to url, records the full CDP
// event stream (including Web Vitals), and returns a short status string.
func toolBrowserNavigate(ctx context.Context, url, step, waitSecStr, saveBaselineStr string) (string, error) {
	// Defense-in-depth preflight. Chrome resolves independently, so this blocks
	// known private/literal/DNS destinations but is not equivalent to dial pinning.
	if err := egress.Check(ctx, url, egress.Policy{}); err != nil {
		return "", fmt.Errorf("sin_browser_navigate: destination denied: %w", err)
	}

	waitSec := 3
	if waitSecStr != "" {
		var n int
		if _, err := fmt.Sscanf(waitSecStr, "%d", &n); err == nil && n >= 0 && n <= 120 {
			waitSec = n
		}
	}

	if browserSession != nil {
		_ = browserSession.rec.Close()
		browserSession.cancelCtx()
		browserSession = nil
	}

	jsonlPath := filepath.Join(os.TempDir(), fmt.Sprintf("sin-browser-%d.jsonl", time.Now().UnixNano()))

	cfg := cdp.DefaultConfig(jsonlPath)
	rec, err := cdp.NewRecorder(cfg)
	if err != nil {
		return "", fmt.Errorf("sin_browser_navigate: failed to create recorder: %w", err)
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx,
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-gpu", true),
		)...)

	cdpCtx, cancelCDP := chromedp.NewContext(allocCtx)

	rec.Attach(cdpCtx)
	if err := rec.EnableDomains(cdpCtx); err != nil {
		cancelCDP()
		cancelAlloc()
		_ = rec.Close()
		return "", fmt.Errorf("sin_browser_navigate: enable domains: %w", err)
	}
	if err := rec.InstallVitals(cdpCtx); err != nil {
		_ = err
	}

	if step != "" {
		rec.SetStep(step)
	} else {
		rec.SetStep("navigate")
	}

	navErr := chromedp.Run(cdpCtx,
		chromedp.Navigate(url),
		chromedp.Sleep(time.Duration(waitSec)*time.Second),
	)

	rec.SetStep("")

	combinedCancel := func() { cancelCDP(); cancelAlloc() }
	var prevBaseline *cdp.Report
	if browserSession != nil {
		prevBaseline = browserSession.baseline
	}
	browserSession = &activeBrowserSession{
		rec:       rec,
		cancelCtx: combinedCancel,
		jsonlPath: jsonlPath,
		cdpCtx:    cdpCtx,
		baseline:  prevBaseline,
	}
	if saveBaselineStr == "true" {
		browserSession.baseline = cdp.BuildReport(rec.Events(), 25)
	}

	events := rec.Events()
	if navErr != nil {
		return fmt.Sprintf("navigation error: %v  (captured %d events, JSONL: %s)", navErr, len(events), jsonlPath), nil
	}

	counts := map[string]int{}
	for _, e := range events {
		counts[e.Domain]++
	}
	b, _ := json.MarshalIndent(counts, "", "  ")
	return fmt.Sprintf("navigated to %s — %d events captured\nJSONL: %s\ndomains: %s",
		url, len(events), jsonlPath, string(b)), nil
}

// toolBrowserFindings runs the full deterministic analysis pipeline over the
// last recorded session and returns a structured Report as JSON.
func toolBrowserFindings() (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_findings: no active browser session — call sin_browser_navigate first")
	}
	report := cdp.BuildReport(browserSession.rec.Events(), 25)
	if report.Summary.Errors == 0 && report.Summary.Warnings == 0 {
		b, _ := json.MarshalIndent(report, "", "  ")
		return fmt.Sprintf("no errors or warnings detected\n%s", string(b)), nil
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sin_browser_findings: marshal: %w", err)
	}
	return fmt.Sprintf("report: %d error(s), %d warning(s), %d suggestion(s), fatal=%v\n%s",
		report.Summary.Errors, report.Summary.Warnings,
		len(report.Suggestions), report.Summary.HasFatal,
		string(b)), nil
}

// toolBrowserSnapshot returns a compact JSON summary of the last session.
func toolBrowserSnapshot() (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_snapshot: no active browser session — call sin_browser_navigate first")
	}
	events := browserSession.rec.Events()
	if len(events) == 0 {
		return `{"total_events":0}`, nil
	}

	report := cdp.BuildReport(events, 25)

	counts := map[string]int{}
	for _, e := range events {
		counts[e.Domain+"."+e.Method]++
	}

	snap := map[string]interface{}{
		"total_events": len(events),
		"first_wall":   events[0].WallTime,
		"last_wall":    events[len(events)-1].WallTime,
		"event_counts": counts,
		"report":       report,
		"jsonl":        browserSession.jsonlPath,
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sin_browser_snapshot: marshal: %w", err)
	}
	return string(b), nil
}

// toolBrowserVitalsFlush forces a final Web Vitals metric flush in the live
// browser tab.
func toolBrowserVitalsFlush(ctx context.Context) (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_vitals_flush: no active browser session — call sin_browser_navigate first")
	}
	browserSession.rec.EvalVitalsNow(browserSession.cdpCtx)
	return "vitals flushed — call sin_browser_findings to get updated metrics", nil
}

// toolBrowserDiff compares the saved baseline Report with the current
// session's Report and returns a Diff.
func toolBrowserDiff(windowStr string) (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_diff: no active browser session — call sin_browser_navigate first")
	}
	if browserSession.baseline == nil {
		return "", fmt.Errorf("sin_browser_diff: no baseline saved — navigate with save_baseline=true first")
	}

	window := uint64(25)
	if windowStr != "" {
		var n uint64
		if _, err := fmt.Sscanf(windowStr, "%d", &n); err == nil && n > 0 && n <= 1000 {
			window = n
		}
	}

	after := cdp.BuildReport(browserSession.rec.Events(), window)
	diff := cdp.DiffReports(browserSession.baseline, after)

	b, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sin_browser_diff: marshal: %w", err)
	}
	verdict := "no improvement (errors unchanged)"
	if diff.Improved {
		verdict = "improved"
	} else if len(diff.Introduced) > 0 {
		verdict = "regression introduced"
	}
	return fmt.Sprintf("diff: %s — resolved=%d introduced=%d persisted=%d before_errors=%d after_errors=%d\n%s",
		verdict,
		len(diff.Resolved), len(diff.Introduced), len(diff.Persisted),
		diff.BeforeErr, diff.AfterErr,
		string(b)), nil
}
