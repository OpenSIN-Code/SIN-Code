// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when tools are MCP-externalized
// Purpose: extended builtin tool registry and central dispatch. Tool
// implementations live in focused files (chat_tools_git.go,
// chat_tools_http.go, chat_tools_testing.go, chat_tools_quality.go,
// chat_tools_browser.go). This file retains the spec table, the dispatch
// switch, shared constants, and spawn_subgoal.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
)

const (
	maxHTTPBytes = 256 * 1024
	gitTimeout   = 30 * time.Second
	testTimeout  = 5 * time.Minute
)

// extraToolFn is injected by coverage tests to mock the dispatch.
var extraToolFn = extraTool

// extraSpecs is appended to builtinSpecs() in chat_tools.go.
func extraSpecs() []agentloopToolSpecAlias {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	obj := func(p map[string]any, req ...string) map[string]any {
		return map[string]any{"type": "object", "properties": p, "required": req}
	}
	allSpecs := []agentloopToolSpecAlias{
		{Name: "sin_git_log", Description: "Show recent commit history (read-only).",
			InputSchema: obj(map[string]any{"limit": str("number of commits, default 10"), "path": str("optional path filter")})},
		{Name: "sin_git_diff", Description: "Show working tree diff or diff vs a ref (read-only).",
			InputSchema: obj(map[string]any{"ref": str("optional ref to diff against, default working tree")})},
		{Name: "sin_git_commit", Description: "Stage all changes and commit with a message (mutating — gated).",
			InputSchema: obj(map[string]any{"message": str("conventional commit message")}, "message")},
		{Name: "sin_http_get", Description: "Fetch a URL (GET only, 256KB cap, 30s timeout). For docs/APIs.",
			InputSchema: obj(map[string]any{"url": str("http(s) URL")}, "url")},
		{Name: "sin_web_search", Description: "Search the web using multiple providers (DuckDuckGo free, Tavily AI, SerpAPI, Brave). Returns ranked results with title, URL, snippet, and source. DuckDuckGo works with zero API keys. Set WEBSEARCH_TAVILY_KEY, WEBSEARCH_SERPAPI_KEY, WEBSEARCH_BRAVE_KEY for additional providers.",
			InputSchema: obj(map[string]any{
				"query": str("search query string"),
				"max":   str("max results (default 10)"),
				"json":  str("emit structured JSON (default false = human-readable)"),
			}, "query")},
		{Name: "sin_test", Description: "Run the workspace test suite with race detection and coverage, returning structured pass/fail output. Set json=true for machine-readable output.",
			InputSchema: obj(map[string]any{
				"target":  str("optional package/file filter (default ./...)"),
				"race":    str("run with -race (default true)"),
				"cover":   str("run with -coverprofile (default true)"),
				"json":    str("emit structured JSON instead of plain text (default false)"),
				"timeout": str("go test timeout, e.g. 5m (default 5m)"),
			})},
		{Name: "sin_test_generate", Description: "Generate table-driven Go tests for a file or package. Uses gotests if available; otherwise falls back to a pure-stdlib generator. Verifies the generated tests compile. Writes files, so gated by permission engine.",
			InputSchema: obj(map[string]any{
				"file":      str("single .go file to generate tests for"),
				"package":   str("package pattern to generate tests for (default ./..., ignored if file is set)"),
				"overwrite": str("replace existing test files (default false)"),
				"llm":       str("use LLM to fill test cases (default false)"),
			})},
		{Name: "sin_quality_gate", Description: "Run the quality gate pipeline: go build, go vet, go test -race -cover, and optional staticcheck/gosec/govulncheck if on PATH. Returns a structured PASS/FAIL report with coverage. Set json=true for machine-readable output.",
			InputSchema: obj(map[string]any{
				"coverage": str("minimum coverage percent required (default 0 = disabled)"),
				"timeout":  str("pipeline timeout, e.g. 5m (default 5m)"),
				"json":     str("emit structured JSON instead of plain text (default false)"),
				"steps":    str("comma-separated steps to run (default all: build,vet,test,staticcheck,gosec,govulncheck)"),
				"race":     str("run go test with -race (default true)"),
			})},
		{Name: "sin_mutation", Description: "Run mutation testing with gremlins if available on PATH. Returns a structured report with mutation score. Set json=true for machine-readable output.",
			InputSchema: obj(map[string]any{
				"threshold": str("minimum mutation score percent required (default 0 = disabled)"),
				"package":   str("package pattern to mutate (default ./...)"),
				"timeout":   str("timeout, e.g. 10m (default 10m)"),
				"json":      str("emit structured JSON instead of plain text (default false)"),
			})},
		{Name: "sin_fuzz", Description: "Run native Go fuzz targets for a package or file. Returns a structured report with fuzzing results.",
			InputSchema: obj(map[string]any{
				"package":  str("package pattern containing fuzz targets (default ./...)"),
				"duration": str("fuzz duration, e.g. 30s (default 30s)"),
				"timeout":  str("overall timeout, e.g. 5m (default 5m)"),
				"json":     str("emit structured JSON instead of plain text (default false)"),
			})},
		{Name: "sin_property", Description: "Run property-based tests via rapid or testing/quick if available. Returns a structured report.",
			InputSchema: obj(map[string]any{
				"package": str("package pattern containing property tests (default ./...)"),
				"timeout": str("timeout, e.g. 5m (default 5m)"),
				"json":    str("emit structured JSON instead of plain text (default false)"),
			})},
		{Name: "sin_dodone_check", Description: "Run a deterministic Definition-of-Done check: placeholder scan, error paths, tests, build, artifacts, requirements coverage, dead code. Returns a structured PASS/FAIL report per pillar. Use before claiming 'done'.",
			InputSchema: obj(map[string]any{
				"task":           str("task description (default: current workspace)"),
				"required_files": str("comma-separated files that must exist (default: README.md)"),
				"requirements":   str("comma-separated requirements to verify (agent provides file:line evidence)"),
				"skip_tests":     str("skip test execution (default false)"),
				"skip_build":     str("skip build/vet (default false)"),
				"json":           str("emit structured JSON (default false)"),
			})},
		// Browser CDP tools — headless Chrome via Chrome DevTools Protocol.
		{Name: "sin_browser_navigate", Description: "Navigate headless Chrome to a URL and record the full CDP event stream (network, console, exceptions, DevTools Audits, security, Web Vitals). Returns event counts. Call sin_browser_findings after to get the full Report. Set save_baseline=true before applying a fix so sin_browser_diff can compare before/after.",
			InputSchema: obj(map[string]any{
				"url":           str("http(s) URL to navigate to"),
				"step":          str("optional label for correlation (e.g. 'login_submit')"),
				"wait_sec":      str("seconds to wait after navigation (default 3)"),
				"save_baseline": str("set to 'true' to save this session's Report as the baseline for sin_browser_diff"),
			}, "url")},
		{Name: "sin_browser_findings", Description: "Return a full structured Report from the last sin_browser_navigate session: classified Findings (network/console/exception/audit/security/vital), root-cause Chains, FixSuggestions with FixClass routing tags, and a Summary (errors/warnings/has_fatal). Use this instead of reading raw JSONL.",
			InputSchema: obj(map[string]any{})},
		{Name: "sin_browser_snapshot", Description: "Return a compact JSON summary of the last sin_browser_navigate session: event counts by domain, first/last wall times, finding count, and the full Report. Useful for a quick health check before calling sin_browser_diff.",
			InputSchema: obj(map[string]any{})},
		{Name: "sin_browser_vitals_flush", Description: "Force a final Web Vitals metric flush in the current browser tab so that LCP/CLS/INP values are captured before calling sin_browser_findings. Call this right before sin_browser_findings when the page is already loaded.",
			InputSchema: obj(map[string]any{})},
		{Name: "sin_browser_diff", Description: "Compare two browser sessions — the stored baseline (saved by the last sin_browser_navigate with save_baseline=true) with the current session — and return a Diff: resolved, introduced, and persisted Findings plus an 'improved' flag. Use after applying a fix to verify it worked.",
			InputSchema: obj(map[string]any{
				"window": str("correlation window in sequence steps for the current session (default 25)"),
			})},
	}
	allSpecs = append(allSpecs, registerBrowserInteractionSpecs()...)
	allSpecs = append(allSpecs, agentloop.SpawnSubgoalSpec())
	return allSpecs
}

// extraTool is called from builtinTool()'s default branch.
func extraTool(ctx context.Context, name string, args map[string]any) (string, error) {
	if out, handled, err := dispatchBrowserInteraction(ctx, name, args); handled {
		return out, err
	}
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
		return runGitFn(ctx, a...)
	case "sin_git_diff":
		if ref := argStr(args, "ref"); ref != "" {
			return runGitFn(ctx, "diff", ref, "--stat", "-p")
		}
		return runGitFn(ctx, "diff", "--stat", "-p")
	case "sin_git_commit":
		msg := argStr(args, "message")
		if msg == "" {
			return "", fmt.Errorf("sin_git_commit: message required")
		}
		if out, err := runGitFn(ctx, "add", "-A"); err != nil {
			return out, err
		}
		return runGitFn(ctx, "commit", "-m", msg)
	case "sin_http_get":
		return toolHTTPGetFn(ctx, argStr(args, "url"))
	case "sin_web_search":
		return toolWebSearch(ctx, args)
	case "sin_test":
		return toolTestFn(ctx, argStr(args, "target"))
	case "sin_test_generate":
		return toolTestGenerate(ctx, args)
	case "sin_quality_gate":
		return toolQualityGate(ctx, args)
	case "sin_mutation":
		return toolMutation(ctx, args)
	case "sin_fuzz":
		return toolFuzz(ctx, args)
	case "sin_property":
		return toolProperty(ctx, args)
	case "sin_dodone_check":
		return toolDodoneCheck(ctx, args)
	case "sin_browser_navigate":
		return toolBrowserNavigate(ctx, argStr(args, "url"), argStr(args, "step"), argStr(args, "wait_sec"), argStr(args, "save_baseline"))
	case "sin_browser_findings":
		return toolBrowserFindings()
	case "sin_browser_snapshot":
		return toolBrowserSnapshot()
	case "sin_browser_vitals_flush":
		return toolBrowserVitalsFlush(ctx)
	case "sin_browser_diff":
		return toolBrowserDiff(argStr(args, "window"))
	case "spawn_subgoal":
		return toolSpawnSubgoal(ctx, args)
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// toolSpawnSubgoal handles the spawn_subgoal tool from the chat surface.
// The chat tool function is stateless (no autonomy queue access); actual
// sub-goal enqueueing happens through the daemon's wrapWithSpawn wrapper
// (commands.go) which captures the queue via closure. When called from
// the chat surface (non-daemon), the handler explains the limitation.
//
// sin-debt: chat-surface wiring, upgrade: when chat_cmd gains queue access
func toolSpawnSubgoal(ctx context.Context, args map[string]any) (string, error) {
	desc := argStr(args, "description")
	if desc == "" {
		return "", fmt.Errorf("spawn_subgoal: 'description' argument is required")
	}
	return "spawn_subgoal is available in daemon mode (sin-code daemon). " +
		"In interactive chat, decompose the task inline instead.", nil
}
