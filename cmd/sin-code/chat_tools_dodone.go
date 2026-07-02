// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: wire into stopgate when Goal-Contract matures
// Purpose: sin_dodone_check — deterministic Definition-of-Done verifier.
// Runs 7 pillars (placeholder scan, error paths, tests, build, artifacts,
// requirements coverage, dead code) and returns a structured PASS/FAIL report.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// dodPillarResult holds the outcome of a single DoD pillar.
type dodPillarResult struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"` // PASS, FAIL, SKIP
	Details string   `json:"details,omitempty"`
	Items   []string `json:"items,omitempty"`
}

// dodReport is the full structured output of sin_dodone_check.
type dodReport struct {
	Task     string          `json:"task"`
	ExitCode int             `json:"exit_code"`
	Pillars  []dodPillarResult `json:"pillars"`
	Summary  string          `json:"summary"`
}

// forbiddenGoPatterns are placeholder/stub patterns to detect in Go source.
var forbiddenGoPatterns = []string{
	"TODO", "FIXME", "panic(",
	"// Hier Logik einfügen", "// implement", "// stub",
}

// forbiddenPyPatterns are placeholder/stub patterns to detect in Python source.
var forbiddenPyPatterns = []string{
	"TODO", "FIXME", "pass  #", "NotImplemented",
	"raise NotImplementedError",
}

// errorIgnorePatterns detect swallowed errors.
var errorIgnorePatterns = []struct {
	Pattern string
	Lang    string
}{
	{"_ = err", "go"},
	{"except.*:\\s*pass", "python"},
	{"catch.*{\\s*}", "javascript"},
}

// toolDodoneCheck is the implementation of sin_dodone_check.
// It runs a deterministic DoD check pipeline and returns a structured report.
func toolDodoneCheck(ctx context.Context, args map[string]any) (string, error) {
	task := argStr(args, "task")
	if task == "" {
		task = "current workspace"
	}
	reqFilesStr := argStr(args, "required_files")
	reqFiles := []string{"README.md"}
	if reqFilesStr != "" {
		reqFiles = strings.Split(reqFilesStr, ",")
	}
	reqsStr := argStr(args, "requirements")
	var requirements []string
	if reqsStr != "" {
		requirements = strings.Split(reqsStr, ",")
	}
	skipTests := argStr(args, "skip_tests") == "true"
	skipBuild := argStr(args, "skip_build") == "true"
	jsonOut := argStr(args, "json") == "true"

	ws, _ := os.Getwd()
	var pillars []dodPillarResult

	// P1: No forbidden patterns (placeholders/stubs)
	pillars = append(pillars, dodCheckPlaceholders(ws))

	// P2: Error handling — no swallowed errors
	pillars = append(pillars, dodCheckErrorPaths(ws))

	// P3: Tests pass
	if skipTests {
		pillars = append(pillars, dodPillarResult{Name: "tests", Status: "SKIP", Details: "skipped via skip_tests=true"})
	} else {
		pillars = append(pillars, dodCheckTests(ctx, ws))
	}

	// P4: Build + vet
	if skipBuild {
		pillars = append(pillars, dodPillarResult{Name: "build+lint", Status: "SKIP", Details: "skipped via skip_build=true"})
	} else {
		pillars = append(pillars, dodCheckBuild(ctx, ws))
	}

	// P5: Required artifacts
	pillars = append(pillars, dodCheckArtifacts(ws, reqFiles))

	// P6: Requirements coverage (agent must provide evidence)
	pillars = append(pillars, dodCheckRequirements(requirements))

	// P7: Dead code (unused imports via go vet)
	pillars = append(pillars, dodCheckDeadCode(ctx, ws))

	// Compute exit code
	exitCode := 0
	failCount := 0
	for _, p := range pillars {
		if p.Status == "FAIL" {
			failCount++
			exitCode = 2
		}
	}
	// Exit 3 = test/build failures specifically
	for _, p := range pillars {
		if p.Status == "FAIL" && (p.Name == "tests" || p.Name == "build+lint") {
			exitCode = 3
		}
	}
	if failCount == 0 {
		exitCode = 0
	}

	summary := fmt.Sprintf("DoD Check: %d/7 pillars passed", 7-failCount)
	if failCount == 0 {
		summary = "WIRKLICH FERTIG — alle 7 Säulen bestanden"
	}

	if jsonOut {
		report := dodReport{Task: task, ExitCode: exitCode, Pillars: pillars, Summary: summary}
		return marshalJSON(report)
	}

	// Human-readable output
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "====================================================\n")
	fmt.Fprintf(&buf, "    SIN-DoDone v2 — Definition-of-Done Check       \n")
	fmt.Fprintf(&buf, "====================================================\n")
	fmt.Fprintf(&buf, "Task: %s\n\n", task)

	for _, p := range pillars {
		icon := "[PASS]"
		if p.Status == "FAIL" {
			icon = "[FAIL]"
		} else if p.Status == "SKIP" {
			icon = "[SKIP]"
		}
		fmt.Fprintf(&buf, "%s %s: %s\n", icon, p.Name, p.Status)
		if p.Details != "" {
			fmt.Fprintf(&buf, "  %s\n", p.Details)
		}
		for _, item := range p.Items {
			fmt.Fprintf(&buf, "    %s\n", item)
		}
		fmt.Fprintf(&buf, "\n")
	}

	fmt.Fprintf(&buf, "====================================================\n")
	fmt.Fprintf(&buf, "%s\n", summary)
	if exitCode != 0 {
		fmt.Fprintf(&buf, "Exit-Code: %d (0=done, 2=code incomplete, 3=test/build failed)\n", exitCode)
	}

	return buf.String(), nil
}

// --- Pillar implementations ---

func dodCheckPlaceholders(ws string) dodPillarResult {
	var items []string
	langs := detectLangs(ws)
	patterns := forbiddenGoPatterns
	if langs["python"] && !langs["go"] {
		patterns = forbiddenPyPatterns
	}

	for _, pat := range patterns {
		matches := grepWorkspace(ws, pat, langExts(langs))
		for _, m := range matches {
			items = append(items, fmt.Sprintf("pattern %q: %s", pat, m))
		}
	}

	status := "PASS"
	if len(items) > 0 {
		status = "FAIL"
	}
	return dodPillarResult{Name: "no_placeholders", Status: status, Items: items}
}

func dodCheckErrorPaths(ws string) dodPillarResult {
	var items []string
	for _, ip := range errorIgnorePatterns {
		matches := grepWorkspace(ws, ip.Pattern, nil)
		for _, m := range matches {
			items = append(items, fmt.Sprintf("[%s] %s", ip.Lang, m))
		}
	}
	status := "PASS"
	if len(items) > 0 {
		status = "FAIL"
	}
	return dodPillarResult{Name: "error_handling", Status: status, Items: items}
}

func dodCheckTests(ctx context.Context, ws string) dodPillarResult {
	langs := detectLangs(ws)
	if !langs["go"] && !langs["python"] && !langs["node"] && !langs["rust"] {
		return dodPillarResult{Name: "tests", Status: "SKIP", Details: "no test framework detected"}
	}

	var cmd *exec.Cmd
	switch {
	case langs["go"]:
		cmd = exec.CommandContext(ctx, "go", "test", "./...", "-v", "-count=1", "-race")
	case langs["python"]:
		cmd = exec.CommandContext(ctx, "pytest", "-v", "--tb=short")
	case langs["node"]:
		cmd = exec.CommandContext(ctx, "npm", "test")
	case langs["rust"]:
		cmd = exec.CommandContext(ctx, "cargo", "test", "--", "--nocapture")
	}
	cmd.Dir = ws
	out, err := cmd.CombinedOutput()
	if err != nil {
		return dodPillarResult{Name: "tests", Status: "FAIL", Details: fmt.Sprintf("test command failed: %v", err), Items: truncateLines(string(out), 10)}
	}
	return dodPillarResult{Name: "tests", Status: "PASS", Details: "test suite green"}
}

func dodCheckBuild(ctx context.Context, ws string) dodPillarResult {
	langs := detectLangs(ws)
	var items []string

	// Build
	var buildCmd *exec.Cmd
	switch {
	case langs["go"]:
		buildCmd = exec.CommandContext(ctx, "go", "build", "./...")
	case langs["rust"]:
		buildCmd = exec.CommandContext(ctx, "cargo", "build")
	}
	if buildCmd != nil {
		buildCmd.Dir = ws
		if out, err := buildCmd.CombinedOutput(); err != nil {
			items = append(items, fmt.Sprintf("build failed: %v", err))
			items = append(items, truncateLines(string(out), 5)...)
		}
	}

	// Vet/lint
	var vetCmd *exec.Cmd
	switch {
	case langs["go"]:
		vetCmd = exec.CommandContext(ctx, "go", "vet", "./...")
	case langs["python"]:
		if _, err := exec.LookPath("ruff"); err == nil {
			vetCmd = exec.CommandContext(ctx, "ruff", "check", ".")
		}
	}
	if vetCmd != nil {
		vetCmd.Dir = ws
		if out, err := vetCmd.CombinedOutput(); err != nil {
			items = append(items, fmt.Sprintf("lint failed: %v", err))
			items = append(items, truncateLines(string(out), 5)...)
		}
	}

	status := "PASS"
	if len(items) > 0 {
		status = "FAIL"
	}
	return dodPillarResult{Name: "build+lint", Status: status, Items: items}
}

func dodCheckArtifacts(ws string, required []string) dodPillarResult {
	var items []string
	for _, f := range required {
		path := filepath.Join(ws, strings.TrimSpace(f))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			items = append(items, fmt.Sprintf("missing: %s", f))
		}
	}
	status := "PASS"
	if len(items) > 0 {
		status = "FAIL"
	}
	return dodPillarResult{Name: "artifacts", Status: status, Items: items}
}

func dodCheckRequirements(requirements []string) dodPillarResult {
	if len(requirements) == 0 {
		return dodPillarResult{Name: "requirements", Status: "SKIP", Details: "no requirements provided"}
	}
	// Requirements need agent-provided evidence; without evidence, flag as FAIL
	var items []string
	for _, r := range requirements {
		items = append(items, fmt.Sprintf("unverified: %s (agent must provide file:line evidence)", strings.TrimSpace(r)))
	}
	return dodPillarResult{Name: "requirements", Status: "FAIL", Items: items}
}

func dodCheckDeadCode(ctx context.Context, ws string) dodPillarResult {
	langs := detectLangs(ws)
	if !langs["go"] {
		return dodPillarResult{Name: "dead_code", Status: "SKIP", Details: "go vet unused-check only available for Go"}
	}
	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	cmd.Dir = ws
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := string(out)
		if strings.Contains(output, "imported and not used") {
			return dodPillarResult{Name: "dead_code", Status: "FAIL", Items: truncateLines(output, 5)}
		}
	}
	return dodPillarResult{Name: "dead_code", Status: "PASS"}
}

// --- Helpers ---

func detectLangs(ws string) map[string]bool {
	langs := map[string]bool{
		"go":     fileExists(filepath.Join(ws, "go.mod")),
		"python": fileExists(filepath.Join(ws, "pyproject.toml")) || fileExists(filepath.Join(ws, "setup.py")),
		"node":   fileExists(filepath.Join(ws, "package.json")),
		"rust":   fileExists(filepath.Join(ws, "Cargo.toml")),
	}
	return langs
}

func langExts(langs map[string]bool) []string {
	var exts []string
	if langs["go"] {
		exts = append(exts, ".go")
	}
	if langs["python"] {
		exts = append(exts, ".py")
	}
	if langs["node"] {
		exts = append(exts, ".js", ".ts", ".tsx")
	}
	if langs["rust"] {
		exts = append(exts, ".rs")
	}
	return exts
}

func grepWorkspace(ws, pattern string, exts []string) []string {
	var results []string
	_ = filepath.Walk(ws, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == ".git" || name == "vendor" || name == "node_modules" {
			return filepath.SkipDir
		}
		if len(exts) > 0 {
			matched := false
			for _, ext := range exts {
				if strings.HasSuffix(name, ext) {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, pattern) {
				rel, _ := filepath.Rel(ws, path)
				results = append(results, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})
	return results
}

func truncateLines(s string, max int) []string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > max {
		lines = lines[:max]
		lines = append(lines, fmt.Sprintf("... (%d more lines)", len(lines)))
	}
	return lines
}

func marshalJSON(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
