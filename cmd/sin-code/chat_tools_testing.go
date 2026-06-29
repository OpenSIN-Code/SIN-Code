// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when tools are MCP-externalized
// Purpose: test tool implementations — sin_test, sin_test_generate, plus
// the auto-generation helpers (maybeGenerateTest). Specs and dispatch
// remain in chat_tools_extra.go.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/testgen"
)

// test hook variables — injected by coverage tests to mock subprocess calls.
var (
	toolTestFn = func(ctx context.Context, target string) (string, error) {
		return toolTest(ctx, map[string]any{"target": target})
	}
	toolTestRunFn = func(cmd *exec.Cmd) ([]byte, error) { return cmd.CombinedOutput() }
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
	if os.Getenv("SIN_TEST_GENERATE_USE_LLM") == "1" {
		useLLM = true
	} else if cfg := testConfig(); cfg.TestUseLLM {
		useLLM = true
	}

	if file == "" && pkg == "" {
		pkg = "./..."
	}

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
type repairLoopOutput struct {
	GeneratedFiles []string `json:"generated_files"`
	TestOutput     string   `json:"test_output"`
	TestPassed     bool     `json:"test_passed"`
	RoundsUsed     int      `json:"rounds_used"`
	CompileErrors  string   `json:"compile_errors,omitempty"`
}

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
			fmt.Fprintf(os.Stderr, "sin_test_generate: LLM fill %q: %v\n", fn.Name, err)
			continue
		}
		key := fn.Name
		if fn.IsMethod {
			key = fn.Receiver + "_" + fn.Name
		}
		casesByFunc[key] = res.Cases
	}
	closure := func(ctx context.Context, code string) (string, error) {
		b, _ := json.Marshal(casesByFunc)
		return string(b), nil
	}
	return casesByFunc, closure
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
