// SPDX-License-Identifier: MIT
// Purpose: extended builtin tools — git operations (read-only allow,
// mutating ask), bounded HTTP fetch, test runner, and browser CDP recording.
// Closes the gap between "needs sin_bash" and "needs a real tool".
//
// Browser tools (sin_browser_*) use pkg/browser/cdp to drive a headless
// Chrome instance via the Chrome DevTools Protocol. They capture the full
// event stream — network, console, exceptions, DevTools Audits (CORS/CSP),
// security state — and surface deterministic Findings so the agent loop can
// act on problems without reading raw JSONL.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/pkg/browser/cdp"
	"github.com/chromedp/chromedp"
)

const (
	maxHTTPBytes = 256 * 1024
	gitTimeout   = 30 * time.Second
	testTimeout  = 5 * time.Minute
)

// extraSpecs is appended to builtinSpecs() in chat_tools.go.
func extraSpecs() []agentloopToolSpecAlias {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	obj := func(p map[string]any, req ...string) map[string]any {
		return map[string]any{"type": "object", "properties": p, "required": req}
	}
	return []agentloopToolSpecAlias{
		{Name: "sin_git_log", Description: "Show recent commit history (read-only).",
			InputSchema: obj(map[string]any{"limit": str("number of commits, default 10"), "path": str("optional path filter")})},
		{Name: "sin_git_diff", Description: "Show working tree diff or diff vs a ref (read-only).",
			InputSchema: obj(map[string]any{"ref": str("optional ref to diff against, default working tree")})},
		{Name: "sin_git_commit", Description: "Stage all changes and commit with a message (mutating — gated).",
			InputSchema: obj(map[string]any{"message": str("conventional commit message")}, "message")},
		{Name: "sin_http_get", Description: "Fetch a URL (GET only, 256KB cap, 30s timeout). For docs/APIs.",
			InputSchema: obj(map[string]any{"url": str("http(s) URL")}, "url")},
		{Name: "sin_test", Description: "Run the workspace test suite and return structured pass/fail output.",
			InputSchema: obj(map[string]any{"target": str("optional package/file filter")})},
		// Browser CDP tools — headless Chrome via Chrome DevTools Protocol.
		// sin_browser_navigate starts a fresh recording session; subsequent
		// sin_browser_findings / sin_browser_snapshot calls consume it.
		{Name: "sin_browser_navigate", Description: "Navigate headless Chrome to a URL and record the full CDP event stream (network, console, exceptions, DevTools Audits, security). Returns a session ID. Call sin_browser_findings after to get classified problems.",
			InputSchema: obj(map[string]any{
				"url":      str("http(s) URL to navigate to"),
				"step":     str("optional label for correlation (e.g. 'login_submit')"),
				"wait_sec": str("seconds to wait after navigation (default 3)"),
			}, "url")},
		{Name: "sin_browser_findings", Description: "Return deterministic classified Findings from the last sin_browser_navigate session: network failures, HTTP errors, JS exceptions, console errors, DevTools Audit issues (CORS/CSP/mixed-content), and security state changes. Sorted by severity then frequency.",
			InputSchema: obj(map[string]any{})},
		{Name: "sin_browser_snapshot", Description: "Return a compact JSON summary of the last sin_browser_navigate session: event counts by domain, first/last wall times, and the raw findings list.",
			InputSchema: obj(map[string]any{})},
	}
}

// extraTool is called from builtinTool()'s default branch.
func extraTool(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case "sin_git_log":
		n := argStr(args, "limit")
		if n == "" {
			n = "10"
		}
		a := []string{"log", "--oneline", "--decorate", "-n", n}
		if p := argStr(args, "path"); p != "" {
			a = append(a, "--", p)
		}
		return runGit(ctx, a...)
	case "sin_git_diff":
		if ref := argStr(args, "ref"); ref != "" {
			return runGit(ctx, "diff", ref, "--stat", "-p")
		}
		return runGit(ctx, "diff", "--stat", "-p")
	case "sin_git_commit":
		msg := argStr(args, "message")
		if msg == "" {
			return "", fmt.Errorf("sin_git_commit: message required")
		}
		if out, err := runGit(ctx, "add", "-A"); err != nil {
			return out, err
		}
		return runGit(ctx, "commit", "-m", msg)
	case "sin_http_get":
		return toolHTTPGet(ctx, argStr(args, "url"))
	case "sin_test":
		return toolTest(ctx, argStr(args, "target"))
	case "sin_browser_navigate":
		return toolBrowserNavigate(ctx, argStr(args, "url"), argStr(args, "step"), argStr(args, "wait_sec"))
	case "sin_browser_findings":
		return toolBrowserFindings()
	case "sin_browser_snapshot":
		return toolBrowserSnapshot()
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func runGit(ctx context.Context, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	if err != nil {
		return fmt.Sprintf("git error: %v\n%s", err, text), nil
	}
	return text, nil
}

func toolHTTPGet(ctx context.Context, url string) (string, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("sin_http_get: only http(s) URLs allowed")
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sin-code-agent/3.5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPBytes))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("HTTP %d (%d bytes)\n%s", resp.StatusCode, len(body), body), nil
}

func toolTest(ctx context.Context, target string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	var cmd *exec.Cmd
	switch {
	case fileExists("go.mod"):
		pkg := "./..."
		if target != "" {
			pkg = target
		}
		cmd = exec.CommandContext(cctx, "go", "test", pkg, "-count=1")
	case fileExists("package.json"):
		cmd = exec.CommandContext(cctx, "sh", "-c", "npm test --silent 2>&1")
	case fileExists("pyproject.toml") || fileExists("pytest.ini"):
		args := []string{"-m", "pytest", "-q"}
		if target != "" {
			args = append(args, target)
		}
		cmd = exec.CommandContext(cctx, "python3", args...)
	default:
		return "", fmt.Errorf("sin_test: no recognized test setup (go.mod/package.json/pyproject.toml)")
	}
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	status := "PASS"
	if err != nil {
		status = "FAIL"
	}
	return fmt.Sprintf("TEST %s\n%s", status, text), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(filepath.Join(".", p))
	return err == nil
}

// silence linter for unused json
var _ = json.Marshal

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
}

// toolBrowserNavigate drives headless Chrome to url, records the full CDP
// event stream, and returns a short status string with event counts.
func toolBrowserNavigate(ctx context.Context, url, step, waitSecStr string) (string, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("sin_browser_navigate: only http(s) URLs are supported")
	}

	// Parse optional wait duration; default 3 s.
	waitSec := 3
	if waitSecStr != "" {
		var n int
		if _, err := fmt.Sscanf(waitSecStr, "%d", &n); err == nil && n >= 0 && n <= 120 {
			waitSec = n
		}
	}

	// Tear down any previous session before starting a new one.
	if browserSession != nil {
		_ = browserSession.rec.Close()
		browserSession.cancelCtx()
		browserSession = nil
	}

	// Write the JSONL to a temp file so it survives the function call.
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

	// Store session for subsequent findings/snapshot calls.
	// combinedCancel releases both the CDP context and the allocator.
	combinedCancel := func() { cancelCDP(); cancelAlloc() }
	browserSession = &activeBrowserSession{
		rec:       rec,
		cancelCtx: combinedCancel,
		jsonlPath: jsonlPath,
	}

	events := rec.Events()
	if navErr != nil {
		return fmt.Sprintf("navigation error: %v  (captured %d events, JSONL: %s)", navErr, len(events), jsonlPath), nil
	}

	// Quick domain breakdown for immediate feedback.
	counts := map[string]int{}
	for _, e := range events {
		counts[e.Domain]++
	}
	b, _ := json.MarshalIndent(counts, "", "  ")
	return fmt.Sprintf("navigated to %s — %d events captured\nJSONL: %s\ndomains: %s",
		url, len(events), jsonlPath, string(b)), nil
}

// toolBrowserFindings runs the deterministic Findings engine over the last
// recorded session and returns the result as a JSON string.
func toolBrowserFindings() (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_findings: no active browser session — call sin_browser_navigate first")
	}
	findings := cdp.Analyze(browserSession.rec.Events())
	if len(findings) == 0 {
		return "no findings — no errors, warnings, or audit issues detected", nil
	}
	b, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sin_browser_findings: marshal: %w", err)
	}
	return fmt.Sprintf("%d finding(s):\n%s", len(findings), string(b)), nil
}

// toolBrowserSnapshot returns a compact summary of the last session without
// running the full Findings engine.
func toolBrowserSnapshot() (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_snapshot: no active browser session — call sin_browser_navigate first")
	}
	events := browserSession.rec.Events()
	if len(events) == 0 {
		return `{"total":0}`, nil
	}

	counts := map[string]int{}
	for _, e := range events {
		counts[e.Domain+"."+e.Method]++
	}

	findings := cdp.Analyze(events)

	snap := map[string]interface{}{
		"total_events": len(events),
		"first_wall":   events[0].WallTime,
		"last_wall":    events[len(events)-1].WallTime,
		"event_counts": counts,
		"finding_count": len(findings),
		"findings":     findings,
		"jsonl":        browserSession.jsonlPath,
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return "", fmt.Errorf("sin_browser_snapshot: marshal: %w", err)
	}
	return string(b), nil
}
