// SPDX-License-Identifier: MIT
// Purpose: `sin-code security scan sca` — vendored SCA scanner integration.
// Locates the sin-sca-go binary (or builds it from the vendored
// SIN-Code-SCA-Tool-Go module), runs it with JSON output, parses the result,
// normalizes findings into the unified SecurityFinding model, and prints them
// in a friendly format.
// Docs: security.doc.md
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

// scanConfig holds runtime options for the SCA scanner.
type scanConfig struct {
	Severity string
	Timeout  int
}

// scaVulnerability mirrors the JSON shape of a single vulnerability from the
// vendored SIN-Code-SCA-Tool-Go scanner.
type scaVulnerability struct {
	ID         string   `json:"id"`
	Package    string   `json:"package"`
	Version    string   `json:"version"`
	Severity   string   `json:"severity"`
	Summary    string   `json:"summary"`
	FixedIn    string   `json:"fixed_in,omitempty"`
	References []string `json:"references,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
}

// scaScanSummary is the severity/count summary reported by the SCA scanner.
type scaScanSummary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Unknown  int `json:"unknown"`
}

// scaRawScanResult mirrors the raw JSON output of the vendored SCA scanner.
type scaRawScanResult struct {
	ProjectPath     string             `json:"project_path"`
	Ecosystem       string             `json:"ecosystem"`
	PackagesScanned int                `json:"packages_scanned"`
	Vulnerabilities []scaVulnerability `json:"vulnerabilities"`
	Summary         scaScanSummary     `json:"summary"`
}

// scaScanResult is the normalized result used by the CLI and JSON output.
type scaScanResult struct {
	ProjectPath     string            `json:"project_path"`
	Ecosystem       string            `json:"ecosystem"`
	PackagesScanned int               `json:"packages_scanned"`
	Vulnerabilities []SecurityFinding `json:"vulnerabilities"`
	Summary         scaScanSummary    `json:"summary"`
}

// NewSecurityScanScaCmd returns the `sin-code security scan sca` subcommand.
func NewSecurityScanScaCmd() *cobra.Command {
	var (
		severity string
		format   string
		strict   bool
		timeout  int
		noBuild  bool
	)

	cmd := &cobra.Command{
		Use:   "sca [path]",
		Short: "Run the vendored SCA scanner on a project",
		Long: `sca runs the vendored SIN-Code-SCA-Tool-Go software composition analysis scanner against the given path.

It parses dependency lock files (go.mod, package-lock.json, requirements.txt, pom.xml)
and queries vulnerabilities via OSV.dev, then prints a concise summary.
JSON output is available for CI.

The scanner binary is located in the following order:
  1. $SIN_SCA_BIN
  2. A binary named "sin-sca-go" on PATH
  3. The vendored SIN-Code-SCA-Tool-Go module (built into the user cache if needed)

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

			bin, err := scaBinLocator(abs, noBuild)
			if err != nil {
				return err
			}

			cfg := scanConfig{
				Severity: severity,
				Timeout:  timeout,
			}
			result, err := runSCAScannerFull(bin, abs, cfg)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if format == "json" {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
			printSCAResult(result)

			if strict && len(result.Vulnerabilities) > 0 {
				return fmt.Errorf("SCA scan found %d vulnerable package(s) (strict mode)", len(result.Vulnerabilities))
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&severity, "severity", "s", "low", "Minimum severity: low, medium, high, critical")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "Output format: text, json")
	cmd.Flags().BoolVar(&strict, "strict", false, "Exit with error if any vulnerabilities are found")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "Timeout per scan in seconds")
	cmd.Flags().BoolVar(&noBuild, "no-build", false, "Do not build the vendored scanner if missing")

	return cmd
}

// scaBinLocator is overridable for tests so they can supply a fake scanner
// without touching PATH or the filesystem.
var scaBinLocator = func(scanPath string, noBuild bool) (string, error) {
	return findSCAScannerBinary(scanPath, noBuild)
}

// findSCAScannerBinary resolves the sin-sca-go binary, building it from the
// vendored module if necessary and allowed.
func findSCAScannerBinary(scanPath string, noBuild bool) (string, error) {
	if env := os.Getenv("SIN_SCA_BIN"); env != "" {
		if fileExists(env) {
			return env, nil
		}
		return "", fmt.Errorf("SIN_SCA_BIN points to missing file: %s", env)
	}

	if p, err := exec.LookPath("sin-sca-go"); err == nil {
		return p, nil
	}

	candidates := []string{scanPath}
	if ex, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(ex))
	}

	for _, start := range candidates {
		moduleDir := findSCAModuleRoot(start)
		if moduleDir == "" {
			continue
		}
		bin := cachedSCABinaryPath()
		if bin == "" {
			return "", fmt.Errorf("cannot determine cache directory for SCA scanner")
		}
		if fileExists(bin) {
			return bin, nil
		}
		if noBuild {
			return "", fmt.Errorf("SCA scanner not found and --no-build prevents build")
		}
		return buildSCAScanner(moduleDir)
	}

	return "", fmt.Errorf("SCA scanner not found; install sin-sca-go or set SIN_SCA_BIN")
}

// findSCAModuleRoot walks upward from start looking for a directory that
// contains SIN-Code-SCA-Tool-Go/go.mod.
func findSCAModuleRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(abs, "SIN-Code-SCA-Tool-Go")
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

// scaCacheDir can be overridden by tests to avoid relying on the user's real cache.
var scaCacheDir = os.UserCacheDir

// cachedSCABinaryPath returns the path where the built scanner binary is cached.
func cachedSCABinaryPath() string {
	dir, err := scaCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "sin-code", "sin-sca-go"+binaryExt())
}

// buildSCAScanner compiles the vendored scanner into the user cache.
func buildSCAScanner(moduleDir string) (string, error) {
	out := cachedSCABinaryPath()
	if out == "" {
		return "", fmt.Errorf("cannot determine cache directory")
	}
	cacheDir := filepath.Dir(out)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create SCA scanner cache dir: %w", err)
	}

	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("go not found in PATH; cannot build SCA scanner")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Remove any stale binary so a failed build is not masked by an old file.
	_ = os.Remove(out)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/sca")
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildOut, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build SCA scanner in %s: %w\n%s", moduleDir, err, string(buildOut))
	}
	return out, nil
}

// runSCAScanner invokes the scanner binary and returns the parsed findings.
// It is the public entry point required by the SCA integration contract.
func runSCAScanner(binary, path string, cfg scanConfig) ([]SecurityFinding, error) {
	result, err := runSCAScannerFull(binary, path, cfg)
	if err != nil {
		return nil, err
	}
	return result.Vulnerabilities, nil
}

// runSCAScannerFull runs the scanner and returns the full normalized result,
// applying any configured severity filter.
func runSCAScannerFull(binary, path string, cfg scanConfig) (*scaScanResult, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout)*time.Second)
	defer cancel()

	args := []string{"-path", path, "-timeout", fmt.Sprintf("%ds", cfg.Timeout)}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, fmt.Errorf("SCA scanner failed: %w\n%s", err, string(out))
		}
		// The scanner may exit non-zero on scan failures; attempt to parse JSON below.
	}

	var raw scaRawScanResult
	jsonBytes := extractJSON(out)
	if jsonBytes == nil {
		if err != nil {
			return nil, fmt.Errorf("SCA scanner failed: %w\n%s", err, string(out))
		}
		return nil, fmt.Errorf("SCA scanner output contains no JSON object\n%s", string(out))
	}
	if jerr := json.Unmarshal(jsonBytes, &raw); jerr != nil {
		if err != nil {
			return nil, fmt.Errorf("SCA scanner failed: %w\n%s", err, string(out))
		}
		return nil, fmt.Errorf("parse SCA scanner output: %w\n%s", jerr, string(out))
	}

	findings := make([]SecurityFinding, 0, len(raw.Vulnerabilities))
	for _, v := range raw.Vulnerabilities {
		findings = append(findings, scaVulnToSecurityFinding(v))
	}

	if cfg.Severity != "" {
		findings = filterSCAFindings(findings, cfg.Severity)
	}

	return &scaScanResult{
		ProjectPath:     raw.ProjectPath,
		Ecosystem:       raw.Ecosystem,
		PackagesScanned: raw.PackagesScanned,
		Vulnerabilities: findings,
		Summary:         summarizeSCAFindings(findings),
	}, nil
}

// extractJSON returns the first JSON object or array found in a byte slice.
// It is tolerant of log lines and other non-JSON text that a vendored scanner
// may print before the JSON payload.
func extractJSON(data []byte) []byte {
	start := -1
	for i, b := range data {
		if b == '{' || b == '[' {
			start = i
			break
		}
	}
	if start == -1 {
		return nil
	}

	end := -1
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == '}' || data[i] == ']' {
			end = i + 1
			break
		}
	}
	if end == -1 || end <= start {
		return nil
	}
	return data[start:end]
}

// scaVulnToSecurityFinding converts a vendored SCA vulnerability into the
// unified SecurityFinding model.
func scaVulnToSecurityFinding(v scaVulnerability) SecurityFinding {
	remediation := ""
	if v.FixedIn != "" {
		remediation = fmt.Sprintf("Upgrade to %s", v.FixedIn)
	}
	return SecurityFinding{
		Scanner:     "sca",
		RuleID:      v.ID,
		Severity:    v.Severity,
		Title:       v.ID,
		Description: v.Summary,
		Package:     v.Package,
		Version:     v.Version,
		Remediation: remediation,
	}
}

// severityRankSca gives a numeric ordering for severity strings (lower = more severe).
func severityRankSca(s string) int {
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

// filterSCAFindings drops findings below the configured severity threshold.
func filterSCAFindings(findings []SecurityFinding, severity string) []SecurityFinding {
	threshold := severityRankSca(severity)
	filtered := make([]SecurityFinding, 0, len(findings))
	for _, f := range findings {
		if severityRankSca(f.Severity) <= threshold {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// summarizeSCAFindings computes a severity summary from a slice of findings.
func summarizeSCAFindings(findings []SecurityFinding) scaScanSummary {
	out := scaScanSummary{
		Total:    len(findings),
		Critical: 0,
		High:     0,
		Medium:   0,
		Low:      0,
		Unknown:  0,
	}
	for _, f := range findings {
		switch severityRankSca(f.Severity) {
		case 0:
			out.Critical++
		case 1:
			out.High++
		case 2:
			out.Medium++
		case 3:
			out.Low++
		default:
			out.Unknown++
		}
	}
	return out
}

// printSCAResult renders an SCA scan in a human-friendly format.
func printSCAResult(r *scaScanResult) {
	fmt.Printf("📦 SCA Scan Results — %s\n", r.ProjectPath)
	fmt.Printf("   Ecosystem: %s | Packages scanned: %d | Vulnerabilities: %d\n",
		r.Ecosystem, r.PackagesScanned, len(r.Vulnerabilities))

	if len(r.Vulnerabilities) == 0 {
		fmt.Println("\n   ✅ No vulnerable dependencies detected")
		return
	}

	fmt.Printf("   Severity: Critical %d | High %d | Medium %d | Low %d | Unknown %d\n\n",
		r.Summary.Critical, r.Summary.High, r.Summary.Medium, r.Summary.Low, r.Summary.Unknown)

	ordered := append([]SecurityFinding(nil), r.Vulnerabilities...)
	sort.Slice(ordered, func(i, j int) bool {
		ri, rj := severityRankSca(ordered[i].Severity), severityRankSca(ordered[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if ordered[i].Package != ordered[j].Package {
			return ordered[i].Package < ordered[j].Package
		}
		return ordered[i].RuleID < ordered[j].RuleID
	})

	fmt.Printf("   🚨 Vulnerabilities (%d)\n", len(ordered))
	fmt.Println(strings.Repeat("-", 60))
	for _, f := range ordered {
		fmt.Printf("\n   [%s] %s — %s %s\n", strings.ToUpper(f.Severity), f.RuleID, f.Package, f.Version)
		if f.Description != "" {
			fmt.Printf("   Summary: %s\n", f.Description)
		}
		if f.Remediation != "" {
			fmt.Printf("   Remediation: %s\n", f.Remediation)
		}
	}
	fmt.Println()
}
