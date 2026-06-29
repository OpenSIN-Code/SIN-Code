// SPDX-License-Identifier: MIT
// Purpose: SAST scan subcommand for `sin-code security scan sast`.
// Runs the vendored SIN-Code-SAST-Tool static analysis scanner, locates or
// builds the binary, and prints findings in text/JSON/SARIF format.
// Docs: security.doc.md
// sin-debt: shrink, upgrade: when a second sast-related function is needed, merge into a shared file
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────────────────────────────────────
// SAST scan
// ─────────────────────────────────────────────────────────────────────────────

// ─── Data models (mirror of SIN-Code-SAST-Tool/pkg/models) ────────────────

type sastScanFinding struct {
	RuleID      string `json:"rule_id"`
	RuleName    string `json:"rule_name"`
	Severity    string `json:"severity"`
	CWE         string `json:"cwe"`
	OWASP       string `json:"owasp"`
	Language    string `json:"language"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Column      int    `json:"column"`
	Match       string `json:"match"`
	Context     string `json:"context"`
	Remediation string `json:"remediation"`
	Confidence  string `json:"confidence"`
	Description string `json:"description"`
}

type sastScanSummary struct {
	Critical       int            `json:"critical"`
	High           int            `json:"high"`
	Medium         int            `json:"medium"`
	Low            int            `json:"low"`
	FilesScanned   int            `json:"files_scanned"`
	LinesScanned   int            `json:"lines_scanned"`
	RulesTriggered int            `json:"rules_triggered"`
	ByLanguage     map[string]int `json:"by_language"`
	ByOWASP        map[string]int `json:"by_owasp"`
}

type sastScanResult struct {
	Path                string            `json:"path"`
	Status              string            `json:"status"`
	Findings            []sastScanFinding `json:"findings"`
	Summary             sastScanSummary   `json:"summary"`
	ScanDurationSeconds float64           `json:"scan_duration_seconds"`
	Timestamp           string            `json:"timestamp"`
}

// NewSecurityScanSastCmd returns the `sin-code security scan sast` subcommand.
func NewSecurityScanSastCmd() *cobra.Command {
	var (
		severity string
		langs    string
		exclude  string
		format   string
		strict   bool
		timeout  int
		noBuild  bool
	)

	cmd := &cobra.Command{
		Use:   "sast [path]",
		Short: "Run the vendored SAST scanner on a codebase",
		Long: `sast runs the vendored SIN-Code-SAST-Tool static analysis scanner against the given path.

It detects 20+ vulnerability categories across 10+ languages (SQL injection,
command injection, XSS, hardcoded secrets, weak crypto, path traversal, etc.)
and prints a concise summary. JSON output is available for CI.

The scanner binary is located in the following order:
  1. $SIN_SAST_BIN
  2. A binary named "sin-sast" on PATH
  3. The vendored SIN-Code-SAST-Tool module (built into the user cache if needed)

Use --no-build to fail fast instead of compiling the vendored scanner.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("resolve scan path: %w", err)
			}

			bin, err := sastBinLocator(abs, noBuild)
			if err != nil {
				return err
			}

			result, err := runSASTScanner(bin, abs, severity, langs, exclude, timeout)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			switch format {
			case "json":
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			case "sarif":
				findings := make([]SecurityFinding, 0, len(result.Findings))
				for _, f := range result.Findings {
					findings = append(findings, sastFindingToSecurity(f))
				}
				findings = normalizeFindingPaths(abs, findings)
				return writeSarif(cmd, findings)
			default:
				printSASTResult(result)
			}

			if strict && result.Summary.Critical > 0 {
				return fmt.Errorf("sast scan found %d critical issue(s) (strict mode)", result.Summary.Critical)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&severity, "severity", "s", "low", "Minimum severity: low, medium, high, critical")
	cmd.Flags().StringVarP(&langs, "languages", "l", "", "Comma-separated languages to scan (e.g. go,python,javascript)")
	cmd.Flags().StringVarP(&exclude, "exclude", "e", "", "Comma-separated patterns to exclude")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "Output format: text, json, sarif")
	cmd.Flags().BoolVar(&strict, "strict", false, "Exit with error if any critical findings are found")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "Timeout per scan in seconds")
	cmd.Flags().BoolVar(&noBuild, "no-build", false, "Do not build the vendored scanner if missing")

	return cmd
}

// sastBinLocator is overridable for tests so they can supply a fake scanner
// without touching PATH or the filesystem.
var sastBinLocator = func(scanPath string, noBuild bool) (string, error) {
	return findSASTScannerBinary(scanPath, noBuild)
}

// findSASTScannerBinary resolves the sin-sast binary, building it from the
// vendored module if necessary and allowed.
func findSASTScannerBinary(scanPath string, noBuild bool) (string, error) {
	if env := os.Getenv("SIN_SAST_BIN"); env != "" {
		if fileExists(env) {
			return env, nil
		}
		return "", fmt.Errorf("SIN_SAST_BIN points to missing file: %s", env)
	}

	if p, err := exec.LookPath("sin-sast"); err == nil {
		return p, nil
	}

	candidates := []string{scanPath}
	if ex, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(ex))
	}

	for _, start := range candidates {
		moduleDir := findSASTModuleRoot(start)
		if moduleDir == "" {
			continue
		}
		bin := cachedSASTBinaryPath()
		if bin == "" {
			return "", fmt.Errorf("cannot determine cache directory for SAST scanner")
		}
		if fileExists(bin) {
			return bin, nil
		}
		if noBuild {
			return "", fmt.Errorf("SAST scanner not found and --no-build prevents build")
		}
		return buildSASTScanner(moduleDir)
	}

	return "", fmt.Errorf("SAST scanner not found; install sin-sast or set SIN_SAST_BIN")
}

// findSASTModuleRoot walks upward from start looking for a directory that
// contains SIN-Code-SAST-Tool/go.mod.
func findSASTModuleRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(abs, "SIN-Code-SAST-Tool")
		if fileExists(filepath.Join(candidate, "go.mod")) {
			return candidate
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	return ""
}

// sastCacheDir can be overridden by tests to avoid relying on the user's real cache.
var sastCacheDir = os.UserCacheDir

// cachedSASTBinaryPath returns the path where the built scanner binary is cached.
func cachedSASTBinaryPath() string {
	dir, err := sastCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "sin-code", "sin-sast"+binaryExt())
}

// buildSASTScanner compiles the vendored scanner into the user cache.
func buildSASTScanner(moduleDir string) (string, error) {
	out := cachedSASTBinaryPath()
	if out == "" {
		return "", fmt.Errorf("cannot determine cache directory")
	}
	cacheDir := filepath.Dir(out)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create SAST scanner cache dir: %w", err)
	}

	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("go not found in PATH; cannot build SAST scanner")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Remove any stale binary so a failed build is not masked by an old file.
	_ = os.Remove(out)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/sin-sast")
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildOut, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build SAST scanner in %s: %w\n%s", moduleDir, err, string(buildOut))
	}
	return out, nil
}

// runSASTScanner invokes the scanner binary and parses its JSON output.
func runSASTScanner(bin, path, severity, langs, exclude string, timeout int) (*sastScanResult, error) {
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	args := []string{"scan", path, "--output", "json", "--severity", severity}
	if langs != "" {
		args = append(args, "--languages", langs)
	}
	if exclude != "" {
		args = append(args, "--exclude", exclude)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, fmt.Errorf("SAST scanner failed: %w\n%s", err, string(out))
		}
		// The scanner exits non-zero when findings are present; keep parsing.
	}

	var result sastScanResult
	if jerr := json.Unmarshal(out, &result); jerr != nil {
		return nil, fmt.Errorf("parse SAST scanner output: %w\n%s", jerr, string(out))
	}
	return &result, nil
}

// printSASTResult renders a SAST scan in a human-friendly format.
func printSASTResult(r *sastScanResult) {
	fmt.Printf("🔍 SAST Scan Results — %s\n", r.Path)
	fmt.Printf("   Duration: %.2fs | Status: %s\n", r.ScanDurationSeconds, strings.ToUpper(r.Status))
	fmt.Printf("   Files scanned: %d | Lines scanned: %d | Rules triggered: %d\n",
		r.Summary.FilesScanned, r.Summary.LinesScanned, r.Summary.RulesTriggered)
	fmt.Printf("   Severity: Critical %d | High %d | Medium %d | Low %d\n",
		r.Summary.Critical, r.Summary.High, r.Summary.Medium, r.Summary.Low)

	if len(r.Findings) == 0 {
		fmt.Println("\n   ✅ No SAST findings detected")
		return
	}

	fmt.Printf("\n   🚨 Findings (%d)\n", len(r.Findings))
	fmt.Println(strings.Repeat("-", 60))

	ordered := append([]sastScanFinding(nil), r.Findings...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Severity != ordered[j].Severity {
			return severityRankSAST(ordered[i].Severity) < severityRankSAST(ordered[j].Severity)
		}
		if ordered[i].File != ordered[j].File {
			return ordered[i].File < ordered[j].File
		}
		return ordered[i].Line < ordered[j].Line
	})

	for _, f := range ordered {
		fmt.Printf("\n   [%s] %s — %s (%s)\n", strings.ToUpper(f.Severity), f.RuleID, f.RuleName, f.Language)
		fmt.Printf("   File: %s:%d\n", f.File, f.Line)
		if f.Match != "" {
			fmt.Printf("   Match: %s\n", f.Match)
		}
		if f.CWE != "" || f.OWASP != "" {
			fmt.Printf("   CWE: %s | OWASP: %s\n", f.CWE, f.OWASP)
		}
		if f.Remediation != "" {
			fmt.Printf("   Remediation: %s\n", f.Remediation)
		}
	}
	fmt.Println()
}

// severityRankSAST gives a numeric ordering for severity strings.
func severityRankSAST(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}
