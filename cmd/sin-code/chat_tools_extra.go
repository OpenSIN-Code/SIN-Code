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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/testgate"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/testgen"
	"github.com/OpenSIN-Code/SIN-Code/pkg/browser/cdp"
)

const (
	maxHTTPBytes = 256 * 1024
	gitTimeout   = 30 * time.Second
	testTimeout  = 5 * time.Minute
)

// extra tool hook variables — injected by coverage tests to mock git, HTTP,
// and test subprocess calls.
var (
	extraToolFn   = extraTool
	runGitFn      = runGit
	toolHTTPGetFn = toolHTTPGet
	toolTestFn    = func(ctx context.Context, target string) (string, error) {
		return toolTest(ctx, map[string]any{"target": target})
	}
	toolHTTPNewRequestFn = http.NewRequestWithContext
	toolHTTPClientDoFn   = func(req *http.Request) (*http.Response, error) { return http.DefaultClient.Do(req) }
	toolTestRunFn        = func(cmd *exec.Cmd) ([]byte, error) { return cmd.CombinedOutput() }
)

var testConfigCache struct {
	once sync.Once
	cfg  internal.SinCodeConfig
	err  error
}

// testConfig returns the merged sin-code config, loading it once. Errors
// degrade to the zero value so tools still work without a config file.
func testConfig() internal.SinCodeConfig {
	testConfigCache.once.Do(func() {
		testConfigCache.cfg, testConfigCache.err = internal.LoadMergedConfig()
	})
	return testConfigCache.cfg
}

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
		// Browser CDP tools — headless Chrome via Chrome DevTools Protocol.
		// sin_browser_navigate starts a fresh recording session; subsequent
		// sin_browser_findings / sin_browser_snapshot calls consume it.
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
	req, err := toolHTTPNewRequestFn(cctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sin-code-agent/3.5")
	resp, err := toolHTTPClientDoFn(req)
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

func toolTest(ctx context.Context, args map[string]any) (string, error) {
	target := argStr(args, "target")
	raceEnabled := argBool(args, "race", true)
	coverEnabled := argBool(args, "cover", true)
	jsonOut := argBool(args, "json", false)
	timeout := argStr(args, "timeout")
	if timeout == "" {
		timeout = "5m"
	}

	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return "", fmt.Errorf("sin_test: invalid timeout %q", timeout)
	}
	cctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	var cmd *exec.Cmd
	switch {
	case fileExists("go.mod"):
		pkg := "./..."
		if target != "" {
			pkg = target
		}
		goArgs := []string{"test", pkg, "-count=1", "-timeout=" + timeout}
		if raceEnabled {
			goArgs = append(goArgs, "-race")
		}
		if coverEnabled {
			goArgs = append(goArgs, "-coverprofile=.sin-code/coverage.out", "-covermode=atomic")
		}
		cmd = exec.CommandContext(cctx, "go", goArgs...)
	case fileExists("package.json"):
		cmd = exec.CommandContext(cctx, "sh", "-c", "npm test --silent 2>&1")
	case fileExists("pyproject.toml") || fileExists("pytest.ini"):
		pyArgs := []string{"-m", "pytest", "-q"}
		if target != "" {
			pyArgs = append(pyArgs, target)
		}
		cmd = exec.CommandContext(cctx, "python3", pyArgs...)
	default:
		return "", fmt.Errorf("sin_test: no recognized test setup (go.mod/package.json/pyproject.toml)")
	}

	out, err := toolTestRunFn(cmd)
	passed := err == nil
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}

	if jsonOut {
		report := map[string]any{
			"status":      "PASS",
			"target":      target,
			"race":        raceEnabled,
			"cover":       coverEnabled,
			"timeout":     timeout,
			"test_output": text,
		}
		if !passed {
			report["status"] = "FAIL"
		}
		// Best-effort coverage extraction from the textual output.
		if coverEnabled {
			report["coverage"] = extractCoverage(text)
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		return string(b), nil
	}

	status := "PASS"
	if !passed {
		status = "FAIL"
	}
	return fmt.Sprintf("TEST %s\n%s", status, text), nil
}

func toolTestGenerate(ctx context.Context, args map[string]any) (string, error) {
	file := argStr(args, "file")
	pkg := argStr(args, "package")
	overwrite := argBool(args, "overwrite", false)
	useLLM := argBool(args, "llm", false)
	// `useLLM||config.test.use_llm` enables calling the configured LLM to fill
	// realistic test cases when the scaffold is generated. Env override:
	// SIN_TEST_GENERATE_USE_LLM=1. Privacy-first: the env var wins over the
	// caller-supplied flag for headless runs (M4).
	if os.Getenv("SIN_TEST_GENERATE_USE_LLM") == "1" {
		useLLM = true
	} else if cfg := testConfig(); cfg.TestUseLLM {
		useLLM = true
	}

	if file == "" && pkg == "" {
		pkg = "./..."
	}

	// When useLLM is true and a single file is specified, try the
	// RepairLoop path (LLMFiller → compile → test → repair). Fall
	// back to the scaffold path when the LLM client cannot be built
	// or the repair loop encounters an unrecoverable error.
	if useLLM && file != "" {
		if res, ok := tryRepairLoopPath(ctx, file, overwrite); ok {
			b, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	}

	var llmFn func(context.Context, string) (string, error)
	var casesByFunc map[string][]testgen.TestCase
	if useLLM {
		casesByFunc, llmFn = buildTestgenLLMFn(ctx, file)
	}

	res := testgen.Generate(ctx, testgen.Options{
		File:      file,
		Package:   pkg,
		UseLLM:    llmFn,
		Overwrite: overwrite,
		Timeout:   testTimeout,
		Cases:     casesByFunc,
	})

	if res.Error != "" {
		return "", fmt.Errorf("sin_test_generate: %s", res.Error)
	}
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// repairLoopOutput is the JSON-compatible result for the RepairLoop path.
// It mirrors testgen.Result fields so downstream callers see a consistent
// shape regardless of which generation path was taken.
type repairLoopOutput struct {
	GeneratedFiles []string `json:"generated_files"`
	TestOutput     string   `json:"test_output"`
	TestPassed     bool     `json:"test_passed"`
	RoundsUsed     int      `json:"rounds_used"`
	CompileErrors  string   `json:"compile_errors,omitempty"`
}

// tryRepairLoopPath attempts to use the LLMFiller + RepairLoop to
// generate, compile, run, and repair tests for a single file. Returns
// (result, true) on success or (nil, false) when the path is not
// available (missing API key, existing test file without overwrite, or
// a loop error) so the caller falls back to the scaffold path.
func tryRepairLoopPath(ctx context.Context, file string, overwrite bool) (*repairLoopOutput, bool) {
	cfg := testConfig()
	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.LLMAPIKey)
	}
	if apiKey == "" {
		return nil, false
	}

	testFile := strings.TrimSuffix(file, ".go") + "_test.go"
	if !overwrite {
		if _, err := os.Stat(testFile); err == nil {
			return nil, false
		}
	}

	baseURL := strings.TrimSpace(cfg.LLMBaseURL)
	if baseURL == "" {
		baseURL = "https://integrate.api.nvidia.com/v1"
	}
	client := llm.NewClient(baseURL, apiKey)
	filler := testgen.NewLLMFiller(client, cfg.LLMModel)

	repairRounds := cfg.TestRepairRounds
	if repairRounds <= 0 {
		repairRounds = testgen.DefaultRepairRounds
	}

	loop := testgen.NewRepairLoop(filler,
		testgen.WithRepairMaxRounds(repairRounds),
		testgen.WithRepairTimeout(testTimeout))

	res, err := loop.Run(ctx, testgen.RepairRequest{
		SourceFile: file,
		MaxRounds:  repairRounds,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "sin_test_generate: repair loop error: %v\n", err)
		return nil, false
	}

	return &repairLoopOutput{
		GeneratedFiles: []string{testFile},
		TestOutput:     res.TestResults,
		TestPassed:     res.FinalPass,
		RoundsUsed:     res.RoundsUsed,
		CompileErrors:  res.CompileErrors,
	}, true
}

// buildTestgenLLMFn returns the per-file `Cases` map and a
// `UseLLM`-compatible closure. When the API key is missing it returns
// (nil, nil) so the caller falls back to the stub scaffold (graceful
// degradation per M4). Both env and config are honoured. The map is
// keyed by `testKey` (free `Name` or `Receiver_Name` for methods).
func buildTestgenLLMFn(ctx context.Context, file string) (map[string][]testgen.TestCase, func(context.Context, string) (string, error)) {
	cfg := testConfig()
	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.LLMAPIKey)
	}
	if apiKey == "" {
		return nil, nil
	}
	if file == "" {
		return nil, nil
	}
	fns, ferr := testgen.FunctionsFromSource(file)
	if ferr != nil || len(fns) == 0 {
		return nil, nil
	}
	baseURL := strings.TrimSpace(cfg.LLMBaseURL)
	if baseURL == "" {
		baseURL = "https://integrate.api.nvidia.com/v1"
	}
	client := llm.NewClient(baseURL, apiKey)
	model := cfg.LLMModel
	casesByFunc := make(map[string][]testgen.TestCase, len(fns))
	for _, fn := range fns {
		res, err := testgen.FillCasesWithLLM(ctx, client, model, fn, testgen.LLMOpts{MaxRepairIters: 0})
		if err != nil {
			// One bad function does not poison the rest of the batch
			// — log via stderr and let the scaffold fall back to its
			// zero-value case for this function.
			fmt.Fprintf(os.Stderr, "sin_test_generate: LLM fill %q: %v\n", fn.Name, err)
			continue
		}
		// testKey matches the lookup the template does.
		key := fn.Name
		if fn.IsMethod {
			key = fn.Receiver + "_" + fn.Name
		}
		casesByFunc[key] = res.Cases
	}
	// Return a closure so the testgen package can re-extract FuncInfo on
	// its own when generating outside this path (stub-only path uses
	// the casesByFunc map directly; the closure stays for parity).
	closure := func(ctx context.Context, code string) (string, error) {
		b, _ := json.Marshal(casesByFunc)
		return string(b), nil
	}
	return casesByFunc, closure
}

func toolQualityGate(ctx context.Context, args map[string]any) (string, error) {
	covStr := argStr(args, "coverage")
	threshold := testConfig().TestCoverageThreshold
	if covStr != "" {
		v, err := strconv.ParseFloat(covStr, 64)
		if err != nil {
			return "", fmt.Errorf("sin_quality_gate: invalid coverage %q", covStr)
		}
		threshold = v
	}

	timeout := argStr(args, "timeout")
	if timeout == "" {
		timeout = "5m"
	}
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return "", fmt.Errorf("sin_quality_gate: invalid timeout %q", timeout)
	}

	var steps []testgate.StepKind
	if raw := argStr(args, "steps"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			steps = append(steps, testgate.StepKind(p))
		}
	}

	report := testgate.Run(ctx, testgate.Config{
		Workdir:           ".",
		Timeout:           dur,
		CoverageThreshold: threshold,
		Race:              argBool(args, "race", true),
		Steps:             steps,
	})

	jsonOut := argBool(args, "json", false)
	if jsonOut {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "QUALITY GATE %s (coverage=%s, threshold=%.1f%%)\n", report.Status, report.Coverage, report.Threshold)
	for _, s := range report.Steps {
		fmt.Fprintf(&sb, "\n[%s] %s (%s)\n", s.Status, s.Name, s.Duration)
		if s.Error != "" {
			fmt.Fprintf(&sb, "ERROR: %s\n", s.Error)
		}
		if s.Output != "" {
			out := s.Output
			if len(out) > maxToolOutput {
				out = out[:maxToolOutput] + "\n[... truncated]"
			}
			fmt.Fprint(&sb, out)
		}
	}
	return sb.String(), nil
}

func toolMutation(ctx context.Context, args map[string]any) (string, error) {
	pkg := argStr(args, "package")
	if pkg == "" {
		pkg = "./..."
	}
	thresholdStr := argStr(args, "threshold")
	threshold := testConfig().TestMutationThreshold
	if thresholdStr != "" {
		v, err := strconv.ParseFloat(thresholdStr, 64)
		if err != nil {
			return "", fmt.Errorf("sin_mutation: invalid threshold %q", thresholdStr)
		}
		threshold = v
	}
	timeout := argStr(args, "timeout")
	if timeout == "" {
		timeout = "10m"
	}
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return "", fmt.Errorf("sin_mutation: invalid timeout %q", timeout)
	}

	if _, err := exec.LookPath("gremlins"); err != nil {
		return "", fmt.Errorf("sin_mutation: gremlins not found on PATH; install from https://github.com/go-gremlins/gremlins")
	}

	cctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	cmdArgs := []string{"unleash", "--test-cpu=1", pkg}
	if threshold > 0 {
		cmdArgs = append(cmdArgs, fmt.Sprintf("--threshold=%.2f", threshold))
	}
	cmd := exec.CommandContext(cctx, "gremlins", cmdArgs...)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	passed := err == nil
	score := extractMutationScore(text)

	jsonOut := argBool(args, "json", false)
	if jsonOut {
		report := map[string]any{
			"status":    "PASS",
			"package":   pkg,
			"threshold": threshold,
			"score":     score,
			"output":    text,
		}
		if !passed {
			report["status"] = "FAIL"
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		return string(b), nil
	}

	status := "PASS"
	if !passed {
		status = "FAIL"
	}
	return fmt.Sprintf("MUTATION %s (score=%.2f%% threshold=%.2f%%)\n%s", status, score, threshold, text), nil
}

func extractMutationScore(out string) float64 {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Mutation Score") || strings.Contains(line, "score") {
			for _, field := range strings.Fields(line) {
				field = strings.TrimSuffix(strings.TrimSuffix(field, "%"), ".")
				if v, err := strconv.ParseFloat(field, 64); err == nil && v >= 0 && v <= 100 {
					return v
				}
			}
		}
	}
	return 0
}

func toolFuzz(ctx context.Context, args map[string]any) (string, error) {
	pkg := argStr(args, "package")
	if pkg == "" {
		pkg = "./..."
	}
	duration := argStr(args, "duration")
	if duration == "" {
		duration = "30s"
	}
	if _, err := time.ParseDuration(duration); err != nil {
		return "", fmt.Errorf("sin_fuzz: invalid duration %q", duration)
	}
	timeout := argStr(args, "timeout")
	if timeout == "" {
		timeout = "5m"
	}
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return "", fmt.Errorf("sin_fuzz: invalid timeout %q", timeout)
	}

	cctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	cmd := exec.CommandContext(cctx, "go", "test", pkg, "-run=^$", fmt.Sprintf("-fuzz=Fuzz.*"), fmt.Sprintf("-fuzztime=%s", duration))
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	passed := err == nil

	jsonOut := argBool(args, "json", false)
	if jsonOut {
		report := map[string]any{
			"status":   "PASS",
			"package":  pkg,
			"duration": duration,
			"output":   text,
		}
		if !passed {
			report["status"] = "FAIL"
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		return string(b), nil
	}

	status := "PASS"
	if !passed {
		status = "FAIL"
	}
	return fmt.Sprintf("FUZZ %s\n%s", status, text), nil
}

func toolProperty(ctx context.Context, args map[string]any) (string, error) {
	pkg := argStr(args, "package")
	if pkg == "" {
		pkg = "./..."
	}
	timeout := argStr(args, "timeout")
	if timeout == "" {
		timeout = "5m"
	}
	dur, err := time.ParseDuration(timeout)
	if err != nil {
		return "", fmt.Errorf("sin_property: invalid timeout %q", timeout)
	}

	cctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	cmd := exec.CommandContext(cctx, "go", "test", pkg, "-run=TestProperty|TestRapid|TestQuick", "-count=1")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	passed := err == nil

	jsonOut := argBool(args, "json", false)
	if jsonOut {
		report := map[string]any{
			"status":  "PASS",
			"package": pkg,
			"output":  text,
		}
		if !passed {
			report["status"] = "FAIL"
		}
		b, _ := json.MarshalIndent(report, "", "  ")
		return string(b), nil
	}

	status := "PASS"
	if !passed {
		status = "FAIL"
	}
	return fmt.Sprintf("PROPERTY %s\n%s", status, text), nil
}

// extractCoverage tries to pull the total coverage line from `go test -cover` output.
func extractCoverage(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "coverage:") || strings.Contains(line, "of statements") {
			return strings.TrimSpace(line)
		}
	}
	return ""
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
	// cdpCtx is kept alive so EvalVitalsNow can run after navigation completes.
	cdpCtx context.Context
	// baseline is the Report saved from a prior run for DiffReports comparison.
	baseline *cdp.Report
}

// toolBrowserNavigate drives headless Chrome to url, records the full CDP
// event stream (including Web Vitals), and returns a short status string.
func toolBrowserNavigate(ctx context.Context, url, step, waitSecStr, saveBaselineStr string) (string, error) {
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
	// Install PerformanceObserver script so LCP/CLS/INP/LongTask metrics are
	// captured as "__SINCDP_VITAL__"-tagged events on every new document.
	if err := rec.InstallVitals(cdpCtx); err != nil {
		// Non-fatal: vitals injection failing should not abort the session.
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

	// Store session for subsequent findings/snapshot/diff calls.
	// combinedCancel releases both the CDP context and the allocator.
	combinedCancel := func() { cancelCDP(); cancelAlloc() }
	var prevBaseline *cdp.Report
	if browserSession != nil {
		prevBaseline = browserSession.baseline // carry over baseline across navigations
	}
	browserSession = &activeBrowserSession{
		rec:       rec,
		cancelCtx: combinedCancel,
		jsonlPath: jsonlPath,
		cdpCtx:    cdpCtx,
		baseline:  prevBaseline,
	}
	// If save_baseline=true, immediately build and store the baseline Report.
	if saveBaselineStr == "true" {
		browserSession.baseline = cdp.BuildReport(rec.Events(), 25)
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

// toolBrowserFindings runs the full deterministic analysis pipeline over the
// last recorded session and returns a structured Report as JSON. The Report
// includes Findings, root-cause Chains, FixSuggestions, and a Summary.
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

// toolBrowserSnapshot returns a compact JSON summary of the last session using
// BuildReport so the agent gets findings, chains, and suggestions in one call.
func toolBrowserSnapshot() (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_snapshot: no active browser session — call sin_browser_navigate first")
	}
	events := browserSession.rec.Events()
	if len(events) == 0 {
		return `{"total_events":0}`, nil
	}

	report := cdp.BuildReport(events, 25)

	// Add per-method event counts as extra context not in the report itself.
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
// browser tab. Call before toolBrowserFindings when the page is already loaded
// so that final CLS/LCP values are emitted before BuildReport runs.
func toolBrowserVitalsFlush(ctx context.Context) (string, error) {
	if browserSession == nil {
		return "", fmt.Errorf("sin_browser_vitals_flush: no active browser session — call sin_browser_navigate first")
	}
	browserSession.rec.EvalVitalsNow(browserSession.cdpCtx)
	return "vitals flushed — call sin_browser_findings to get updated metrics", nil
}

// toolBrowserDiff compares the saved baseline Report with the current session's
// Report and returns a Diff showing resolved, introduced, and persisted findings.
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

// autoGenerateTests enables the Phase 2 tool.post behaviour: after
// sin_write/sin_edit touch a .go file, sin_test_generate is invoked
// automatically. Default off for performance/privacy; can be overridden by
// SIN_AUTO_GENERATE_TESTS=1 or test.auto_generate=true in config.
var autoGenerateTests = os.Getenv("SIN_AUTO_GENERATE_TESTS") == "1"

func autoGenerateEnabled() bool {
	if autoGenerateTests {
		return true
	}
	return testConfig().TestAutoGenerate
}

// maybeGenerateTest runs sin_test_generate for a freshly edited .go file
// when auto-generation is enabled. It returns a short human-readable note
// that is appended to the tool result.
func maybeGenerateTest(path string) string {
	if !autoGenerateEnabled() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return ""
	}
	res := testgen.Generate(context.Background(), testgen.Options{File: path, Timeout: 30 * time.Second})
	if res.Error != "" {
		return fmt.Sprintf("\n[auto-generate] failed: %s", res.Error)
	}
	return fmt.Sprintf("\n[auto-generate] generated %s (test passed=%v)", strings.Join(res.GeneratedFiles, ", "), res.TestPassed)
}
