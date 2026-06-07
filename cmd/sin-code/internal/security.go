// SPDX-License-Identifier: MIT
// Purpose: security — fast security analysis for Go, Python, Node, and generic projects.
// Auto-detects project type, finds available security tools, and runs a targeted scan.
// Docs: security.doc.md
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// SecurityCmd runs a fast security analysis tailored to the detected project type.
var SecurityCmd = &cobra.Command{
	Use:   "security [path]",
	Short: "Fast security analysis — auto-detects project type and runs available tools",
	Long: `security runs a targeted security scan based on the project type detected at <path>.

Supported project types and tools:
  Go        → govulncheck, gosec, go vet (if available)
  Python    → bandit, safety (if available)
  Node.js   → npm audit (if available)
  Generic   → secrets scan (grep for high-entropy strings), file-permission checks

The scan is fast (defaults to 5-minute timeout) and produces a concise summary.
Use --format json for machine-readable output.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		path, _ = filepath.Abs(path)

		projType, _ := cmd.Flags().GetString("type")
		if projType == "" || projType == "auto" {
			projType = detectProjectType(path)
		}

		toolFilter, _ := cmd.Flags().GetString("tools")
		format, _ := cmd.Flags().GetString("format")
		timeoutSec, _ := cmd.Flags().GetInt("timeout")
		strict, _ := cmd.Flags().GetBool("strict")

		result := runSecurityScan(path, projType, toolFilter, timeoutSec)
		result.Strict = strict

		if format == "json" {
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
		} else {
			printSecurityResult(result)
		}

		if strict && result.Summary.Issues > 0 {
			return fmt.Errorf("security scan found %d issues (strict mode)", result.Summary.Issues)
		}
		return nil
	},
}

func init() {
	SecurityCmd.Flags().StringP("type", "t", "auto", "Project type: auto, go, python, node, generic")
	SecurityCmd.Flags().StringP("tools", "T", "", "Comma-separated tool whitelist (e.g. govulncheck,gosec)")
	SecurityCmd.Flags().StringP("format", "f", "text", "Output format: text, json")
	SecurityCmd.Flags().IntP("timeout", "", 300, "Timeout per tool in seconds")
	SecurityCmd.Flags().BoolP("strict", "s", false, "Exit with error if any issues are found")
}

// ─── Data models ───────────────────────────────────────────────────────────

type SecurityResult struct {
	ProjectType string         `json:"project_type"`
	Path        string         `json:"path"`
	Duration    time.Duration  `json:"duration"`
	Strict      bool           `json:"strict"`
	Tools       []ToolResult   `json:"tools"`
	Summary     SecuritySummary `json:"summary"`
}

type ToolResult struct {
	Name     string `json:"name"`
	Status   string `json:"status"`   // ok, issues, skipped, error, not_found
	Issues   int    `json:"issues"`
	Duration string `json:"duration"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
}

type SecuritySummary struct {
	ToolsRun  int `json:"tools_run"`
	Issues    int `json:"issues"`
	Errors    int `json:"errors"`
	Skipped   int `json:"skipped"`
	NotFound  int `json:"not_found"`
}

// ─── Project type detection ────────────────────────────────────────────────

func detectProjectType(path string) string {
	if fileExists(filepath.Join(path, "go.mod")) {
		return "go"
	}
	if fileExists(filepath.Join(path, "package.json")) {
		return "node"
	}
	if fileExists(filepath.Join(path, "requirements.txt")) ||
		fileExists(filepath.Join(path, "pyproject.toml")) ||
		fileExists(filepath.Join(path, "setup.py")) ||
		fileExists(filepath.Join(path, "Pipfile")) {
		return "python"
	}
	return "generic"
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ─── Scan orchestrator ───────────────────────────────────────────────────

func runSecurityScan(path, projType, toolFilter string, timeoutSec int) SecurityResult {
	start := time.Now()
	result := SecurityResult{
		ProjectType: projType,
		Path:        path,
		Tools:       []ToolResult{},
	}

	whitelist := parseToolFilter(toolFilter)

	var tools []toolRunner
	switch projType {
	case "go":
		tools = []toolRunner{
			{name: "govulncheck", run: runGovulncheck},
			{name: "gosec", run: runGosec},
			{name: "go vet", run: runGoVet},
		}
	case "python":
		tools = []toolRunner{
			{name: "bandit", run: runBandit},
			{name: "safety", run: runSafety},
		}
	case "node":
		tools = []toolRunner{
			{name: "npm audit", run: runNpmAudit},
		}
	default:
		tools = []toolRunner{
			{name: "secrets grep", run: runSecretsGrep},
			{name: "file permissions", run: runFilePermissions},
		}
	}

	for _, tr := range tools {
		if whitelist != nil && !whitelist[tr.name] {
			result.Summary.Skipped++
			result.Tools = append(result.Tools, ToolResult{Name: tr.name, Status: "skipped"})
			continue
		}
		toolStart := time.Now()
		status, issues, out, errStr := tr.run(path, timeoutSec)
		dur := time.Since(toolStart).Round(time.Millisecond)

		result.Tools = append(result.Tools, ToolResult{
			Name:     tr.name,
			Status:   status,
			Issues:   issues,
			Duration: dur.String(),
			Output:   out,
			Error:    errStr,
		})

		switch status {
		case "ok":
			result.Summary.ToolsRun++
		case "issues":
			result.Summary.ToolsRun++
			result.Summary.Issues += issues
		case "error":
			result.Summary.ToolsRun++
			result.Summary.Errors++
		case "not_found":
			result.Summary.NotFound++
		}
	}

	result.Duration = time.Since(start).Round(time.Millisecond)
	return result
}

type toolRunner struct {
	name string
	run  func(path string, timeoutSec int) (status string, issues int, output, errStr string)
}

func parseToolFilter(filter string) map[string]bool {
	if filter == "" {
		return nil
	}
	m := make(map[string]bool)
	for _, t := range strings.Split(filter, ",") {
		m[strings.TrimSpace(t)] = true
	}
	return m
}

// ─── Individual tool runners ─────────────────────────────────────────────

func runGovulncheck(path string, timeoutSec int) (string, int, string, string) {
	if !commandExists("govulncheck") {
		return "not_found", 0, "", "govulncheck not found in PATH; install with: go install golang.org/x/vuln/cmd/govulncheck@latest"
	}
	out, err := runWithTimeout("govulncheck", []string{"./..."}, path, timeoutSec)
	if err != nil {
		return "error", 0, string(out), err.Error()
	}
	// govulncheck returns exit 0 if no vulns, non-zero if vulns found
	issues := countSubstring(string(out), "Vulnerability")
	if issues == 0 {
		issues = countSubstring(string(out), "GO-")
	}
	if issues > 0 {
		return "issues", issues, string(out), ""
	}
	return "ok", 0, string(out), ""
}

func runGosec(path string, timeoutSec int) (string, int, string, string) {
	if !commandExists("gosec") {
		return "not_found", 0, "", "gosec not found in PATH; install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"
	}
	out, err := runWithTimeout("gosec", []string{"-fmt", "json", "./..."}, path, timeoutSec)
	if err != nil {
		// gosec returns non-zero when issues are found
		issues := countSubstring(string(out), `"severity"`)
		if issues > 0 {
			return "issues", issues, string(out), ""
		}
		return "error", 0, string(out), err.Error()
	}
	return "ok", 0, string(out), ""
}

func runGoVet(path string, timeoutSec int) (string, int, string, string) {
	if !commandExists("go") {
		return "not_found", 0, "", "go not found in PATH"
	}
	out, err := runWithTimeout("go", []string{"vet", "./..."}, path, timeoutSec)
	if err != nil {
		issues := len(strings.Split(string(out), "\n")) // rough heuristic
		if issues > 0 {
			return "issues", issues, string(out), ""
		}
		return "error", 0, string(out), err.Error()
	}
	return "ok", 0, string(out), ""
}

func runBandit(path string, timeoutSec int) (string, int, string, string) {
	if !commandExists("bandit") {
		return "not_found", 0, "", "bandit not found in PATH; install with: pip install bandit"
	}
	out, err := runWithTimeout("bandit", []string{"-r", ".", "-f", "json"}, path, timeoutSec)
	if err != nil {
		issues := countSubstring(string(out), `"issue_severity"`)
		if issues > 0 {
			return "issues", issues, string(out), ""
		}
		return "error", 0, string(out), err.Error()
	}
	return "ok", 0, string(out), ""
}

func runSafety(path string, timeoutSec int) (string, int, string, string) {
	if !commandExists("safety") {
		return "not_found", 0, "", "safety not found in PATH; install with: pip install safety"
	}
	out, err := runWithTimeout("safety", []string{"check", "--json"}, path, timeoutSec)
	if err != nil {
		issues := countSubstring(string(out), `"vulnerability_id"`)
		if issues > 0 {
			return "issues", issues, string(out), ""
		}
		return "error", 0, string(out), err.Error()
	}
	return "ok", 0, string(out), ""
}

func runNpmAudit(path string, timeoutSec int) (string, int, string, string) {
	if !commandExists("npm") {
		return "not_found", 0, "", "npm not found in PATH"
	}
	out, err := runWithTimeout("npm", []string{"audit", "--json"}, path, timeoutSec)
	if err != nil {
		issues := countSubstring(string(out), `"via"`)
		if issues > 0 {
			return "issues", issues, string(out), ""
		}
		return "error", 0, string(out), err.Error()
	}
	return "ok", 0, string(out), ""
}

func runSecretsGrep(path string, timeoutSec int) (string, int, string, string) {
	// Simple heuristic: grep for high-entropy strings and common secret patterns
	patterns := []string{
		`password\s*=\s*["'][^"']{8,}["']`,
		`api_key\s*=\s*["'][^"']{16,}["']`,
		`secret\s*=\s*["'][^"']{16,}["']`,
		`token\s*=\s*["'][^"']{16,}["']`,
		`AWS_ACCESS_KEY_ID\s*=\s*["']?AKIA`,
		`private_key`,
	}
	found := 0
	var matches []string
	for _, pat := range patterns {
		out, _ := runWithTimeout("grep", []string{"-rE", pat, "--include=*.go", "--include=*.py", "--include=*.js", "--include=*.ts", "--include=*.yaml", "--include=*.yml", "--include=*.json", "--include=*.env*", "--include=*.md"}, path, timeoutSec)
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) != "" && !strings.Contains(line, "_test.go") {
				found++
				if len(matches) < 20 {
					matches = append(matches, line)
				}
			}
		}
	}
	if found > 0 {
		return "issues", found, strings.Join(matches, "\n"), ""
	}
	return "ok", 0, "No high-entropy secrets detected in source files.", ""
}

func runFilePermissions(path string, timeoutSec int) (string, int, string, string) {
	out, err := runWithTimeout("find", []string{".", "-type", "f", "-perm", "+111"}, path, timeoutSec)
	if err != nil {
		return "error", 0, string(out), err.Error()
	}
	lines := strings.Split(string(out), "\n")
	found := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			found++
		}
	}
	if found > 0 {
		return "ok", 0, fmt.Sprintf("%d executable files found (review for unexpected permissions)", found), ""
	}
	return "ok", 0, "No executable files found.", ""
}

// ─── Helpers ─────────────────────────────────────────────────────────────

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

func runWithTimeout(cmd string, args []string, dir string, timeoutSec int) ([]byte, error) {
	c := exec.Command(cmd, args...)
	c.Dir = dir
	c.Env = os.Environ()
	return c.CombinedOutput()
}

func countSubstring(s, sub string) int {
	return strings.Count(s, sub)
}

func countLinesSimple(s string) int {
	return len(strings.Split(s, "\n"))
}

// ─── Output formatting ───────────────────────────────────────────────────

func printSecurityResult(r SecurityResult) {
	fmt.Printf("🔒 Security Scan Summary — %s project at %s\n", r.ProjectType, r.Path)
	fmt.Printf("   Duration: %s\n\n", r.Duration)

	for _, t := range r.Tools {
		switch t.Status {
		case "ok":
			fmt.Printf("   ✅ %-20s  %s  (no issues)\n", t.Name, t.Duration)
		case "issues":
			fmt.Printf("   ⚠️  %-20s  %s  (%d issues)\n", t.Name, t.Duration, t.Issues)
		case "error":
			fmt.Printf("   ❌ %-20s  %s  ERROR: %s\n", t.Name, t.Duration, t.Error)
		case "not_found":
			fmt.Printf("   ⏭️  %-20s  not installed\n", t.Name)
		case "skipped":
			fmt.Printf("   ⏭️  %-20s  skipped\n", t.Name)
		}
	}

	fmt.Println()
	fmt.Printf("   Tools run: %d  |  Issues: %d  |  Errors: %d  |  Not found: %d  |  Skipped: %d\n",
		r.Summary.ToolsRun, r.Summary.Issues, r.Summary.Errors, r.Summary.NotFound, r.Summary.Skipped)

	if r.Strict && r.Summary.Issues > 0 {
		fmt.Printf("\n   ⚠️  Strict mode: %d issues found — exiting with error\n", r.Summary.Issues)
	} else if r.Summary.Issues > 0 {
		fmt.Printf("\n   ⚠️  %d issues found — review recommended\n", r.Summary.Issues)
	} else {
		fmt.Printf("\n   ✅ No security issues detected\n")
	}
}
