// SPDX-License-Identifier: MIT
// Purpose: consolidated security scan subcommands for `sin-code security scan`.
// Contains the container, SAST, SCA, and SBOM scan constructors and their
// helpers. Kept together so the shared security scanner orchestration surface
// is in one place.
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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/security/sca"
)

// ─────────────────────────────────────────────────────────────────────────────
// Container scan
// ─────────────────────────────────────────────────────────────────────────────

// defaultContainerBaseImage is the lightweight image used to scan a local path
// when no explicit image is provided. The scanners are statically linked
// (CGO_ENABLED=0) so they run in any Linux/Unix container.
const defaultContainerBaseImage = "alpine:latest"

type containerScanResult struct {
	Runtime  string            `json:"runtime"`
	Target   string            `json:"target"`
	Kind     string            `json:"kind"` // image or path
	Scanners []string          `json:"scanners"`
	Findings []SecurityFinding `json:"findings"`
	Summary  containerSummary  `json:"summary"`
}

type containerSummary struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Total    int `json:"total"`
}

// containerExecCommand is the production exec wrapper; tests override it to
// avoid requiring a real container runtime.
var containerExecCommand = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}

// containerScannerResolver is the production resolver for vendored scanner
// binaries; tests override it to inject fakes.
var containerScannerResolver = resolveContainerScanner

// detectContainerScanRuntime is a test hook around the real implementation.
var detectContainerScanRuntime = detectContainerScanRuntimeImpl

// NewSecurityScanContainerCmd returns the `sin-code security scan container`
// subcommand.
func NewSecurityScanContainerCmd() *cobra.Command {
	var (
		runtimeFlag  string
		imageFlag    string
		scannersFlag string
		format       string
		strict       bool
		timeout      int
	)

	cmd := &cobra.Command{
		Use:   "container [image|path]",
		Short: "Scan a container image or local path inside a container runtime",
		Long: `container scans a container image or a local path by running it inside a
container runtime. Apple 'container' is preferred over Docker when both are
available.

The scanner binaries are mounted from the host into the container, so no
scanner needs to be installed inside the image. Supported scanners are:
  secrets  — vendored SIN-Code Secrets Scanner
  sast     — vendored SIN-Code SAST Scanner
  sca      — grype (must be on PATH)

The positional argument is either a container image name or a local path.
Use --image to force image mode. When neither is given, the current
directory is scanned as a path inside a container.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			scanners := parseContainerScanners(scannersFlag)
			if len(scanners) == 0 {
				return fmt.Errorf("--scanners is empty; specify at least one of secrets,sast,sca")
			}

			// Determine target and mode.
			var target, kind string
			if imageFlag != "" {
				target = imageFlag
				kind = "image"
			} else {
				target = "."
				if len(args) > 0 {
					target = args[0]
				}
				// If the target exists as a path, treat it as a path; otherwise treat it as an image.
				if abs, err := filepath.Abs(target); err == nil && fileExists(abs) {
					kind = "path"
					target = abs
				} else {
					kind = "image"
				}
			}

			name, binary, err := detectContainerScanRuntime(runtimeFlag)
			if err != nil {
				return err
			}

			var findings []SecurityFinding
			if kind == "image" {
				findings, err = scanContainerImage(binary, target, scanners, timeout)
			} else {
				findings, err = scanContainerPath(binary, target, scanners, timeout)
			}
			if err != nil {
				return err
			}

			result := containerScanResult{
				Runtime:  name,
				Target:   target,
				Kind:     kind,
				Scanners: scanners,
				Findings: findings,
				Summary:  summarizeContainerFindings(findings),
			}

			out := cmd.OutOrStdout()
			if format == "json" {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(result); err != nil {
					return fmt.Errorf("encode JSON output: %w", err)
				}
			} else {
				printContainerScanResult(result)
			}

			if strict && result.Summary.Total > 0 {
				return fmt.Errorf("container scan found %d security finding(s) (strict mode)", result.Summary.Total)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&runtimeFlag, "runtime", "auto", "Container runtime: auto, container (Apple), docker")
	cmd.Flags().StringVar(&imageFlag, "image", "", "Container image to scan (forces image mode)")
	cmd.Flags().StringVar(&scannersFlag, "scanners", "secrets,sast,sca", "Comma-separated scanners: secrets,sast,sca")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "Output format: text, json")
	cmd.Flags().BoolVar(&strict, "strict", false, "Exit with error if any findings are found")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "Timeout per scanner in seconds")

	return cmd
}

// detectContainerScanRuntimeImpl checks for an available container runtime. If pref is
// "auto" it prefers Apple container over Docker; otherwise it validates the
// requested runtime.
func detectContainerScanRuntimeImpl(pref string) (name, binary string, err error) {
	if pref != "" && pref != "auto" {
		switch pref {
		case "container", "apple":
			if p, err := exec.LookPath("container"); err == nil {
				return "apple", p, nil
			}
			return "", "", fmt.Errorf("Apple container runtime requested but 'container' binary not found in PATH")
		case "docker":
			if p, err := exec.LookPath("docker"); err == nil {
				return "docker", p, nil
			}
			return "", "", fmt.Errorf("Docker runtime requested but 'docker' binary not found in PATH")
		default:
			return "", "", fmt.Errorf("unsupported container runtime %q; use auto, container, or docker", pref)
		}
	}

	if p, err := exec.LookPath("container"); err == nil {
		return "apple", p, nil
	}
	if p, err := exec.LookPath("docker"); err == nil {
		return "docker", p, nil
	}
	return "", "", fmt.Errorf("no container runtime found; install Apple container (https://github.com/apple/container) or Docker")
}

// scanContainerImage scans a container image by running each requested scanner
// inside the image filesystem.
func scanContainerImage(runtime, image string, scanners []string, timeoutSec int) ([]SecurityFinding, error) {
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	timeout := time.Duration(timeoutSec) * time.Second

	var all []SecurityFinding
	for _, scanner := range scanners {
		bin, err := containerScannerResolver(scanner, ".")
		if err != nil {
			return nil, fmt.Errorf("resolve scanner %q: %w", scanner, err)
		}
		args := containerRunArgs(runtime, image, containerVolume{src: bin, dst: "/usr/local/bin/" + scanner})
		args = append(args, "/usr/local/bin/"+scanner, "/")

		findings, err := runContainerScanner(scanner, runtime, args, timeout)
		if err != nil {
			return nil, fmt.Errorf("scan image %q with %s: %w", image, scanner, err)
		}
		all = append(all, findings...)
	}
	return all, nil
}

// scanContainerPath scans a local path by mounting it into a container and
// running the requested scanners inside.
func scanContainerPath(runtime, path string, scanners []string, timeoutSec int) ([]SecurityFinding, error) {
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	timeout := time.Duration(timeoutSec) * time.Second

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve scan path: %w", err)
	}

	var all []SecurityFinding
	for _, scanner := range scanners {
		bin, err := containerScannerResolver(scanner, abs)
		if err != nil {
			return nil, fmt.Errorf("resolve scanner %q: %w", scanner, err)
		}
		args := containerRunArgs(runtime, defaultContainerBaseImage,
			containerVolume{src: bin, dst: "/usr/local/bin/" + scanner},
			containerVolume{src: abs, dst: "/workspace"},
		)
		args = append(args, "/usr/local/bin/"+scanner, "/workspace")

		findings, err := runContainerScanner(scanner, runtime, args, timeout)
		if err != nil {
			return nil, fmt.Errorf("scan path %q with %s: %w", abs, scanner, err)
		}
		all = append(all, findings...)
	}
	return all, nil
}

type containerVolume struct {
	src string
	dst string
}

// containerRunArgs builds the runtime-specific `run` argument slice.
func containerRunArgs(runtime, image string, volumes ...containerVolume) []string {
	// Heuristic: the Apple runtime binary is named "container"; everything else
	// is treated as Docker-compatible.
	isApple := strings.HasSuffix(runtime, "container") || strings.Contains(runtime, "container")

	args := []string{"run", "--rm"}
	for _, v := range volumes {
		if isApple {
			args = append(args, "--volume", v.src+":"+v.dst)
		} else {
			args = append(args, "-v", v.src+":"+v.dst)
		}
	}
	args = append(args, image)
	return args
}

// runContainerScanner executes a scanner inside the container and parses the
// output into unified findings.
func runContainerScanner(tool, runtime string, args []string, timeout time.Duration) ([]SecurityFinding, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := containerExecCommand(ctx, runtime, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, fmt.Errorf("%s run failed: %w\n%s", filepath.Base(runtime), err, string(out))
		}
		// Scanners exit non-zero when findings are present; keep parsing.
	}

	return parseContainerScannerOutput(tool, out)
}

// parseContainerScannerOutput converts a scanner's JSON output into unified
// SecurityFinding structs.
func parseContainerScannerOutput(tool string, out []byte) ([]SecurityFinding, error) {
	switch tool {
	case "secrets":
		return parseContainerSecretsOutput(out)
	case "sast":
		return parseContainerSASTOutput(out)
	case "sca":
		return parseContainerSCAOutput(out)
	default:
		return nil, fmt.Errorf("unsupported container scanner %q", tool)
	}
}

func parseContainerSecretsOutput(out []byte) ([]SecurityFinding, error) {
	var r secretsScanResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse secrets scanner output: %w\n%s", err, string(out))
	}
	findings := make([]SecurityFinding, 0, len(r.Findings))
	for _, f := range r.Findings {
		findings = append(findings, SecurityFinding{
			Kind:        "secret",
			Tool:        "secrets",
			Severity:    f.Severity,
			RuleID:      f.RuleID,
			RuleName:    f.RuleName,
			File:        f.File,
			Line:        f.Line,
			Description: fmt.Sprintf("%s (%s)", f.RuleName, f.SecretType),
			Remediation: f.Remediation,
		})
	}
	return findings, nil
}

func parseContainerSASTOutput(out []byte) ([]SecurityFinding, error) {
	var r sastScanResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse SAST scanner output: %w\n%s", err, string(out))
	}
	findings := make([]SecurityFinding, 0, len(r.Findings))
	for _, f := range r.Findings {
		desc := f.Description
		if desc == "" {
			desc = f.Match
		}
		findings = append(findings, SecurityFinding{
			Kind:        "sast",
			Tool:        "sast",
			Severity:    f.Severity,
			RuleID:      f.RuleID,
			RuleName:    f.RuleName,
			File:        f.File,
			Line:        f.Line,
			Description: desc,
			Remediation: f.Remediation,
			CWE:         f.CWE,
			OWASP:       f.OWASP,
		})
	}
	return findings, nil
}

func parseContainerSCAOutput(out []byte) ([]SecurityFinding, error) {
	var r sca.Result
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse SCA scanner output: %w\n%s", err, string(out))
	}
	findings := make([]SecurityFinding, 0, len(r.Vulnerabilities))
	for _, v := range r.Vulnerabilities {
		remediation := ""
		if len(v.FixedIn) > 0 {
			remediation = "Fixed in: " + strings.Join(v.FixedIn, ", ")
		}
		findings = append(findings, SecurityFinding{
			Kind:        "sca",
			Tool:        "sca",
			Severity:    v.Severity,
			RuleID:      v.ID,
			Description: fmt.Sprintf("%s@%s", v.Package, v.Version),
			Remediation: remediation,
		})
	}
	return findings, nil
}

// resolveContainerScanner locates the binary for a named scanner.
func resolveContainerScanner(scanner, scanPath string) (string, error) {
	switch scanner {
	case "secrets":
		return secretsBinLocator(scanPath, false)
	case "sast":
		return sastBinLocator(scanPath, false)
	case "sca":
		if p, err := exec.LookPath("grype"); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("grype not found in PATH; install from https://github.com/anchore/grype")
	default:
		return "", fmt.Errorf("unsupported scanner %q", scanner)
	}
}

func parseContainerScanners(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func summarizeContainerFindings(findings []SecurityFinding) containerSummary {
	s := containerSummary{Total: len(findings)}
	for _, f := range findings {
		switch strings.ToLower(f.Severity) {
		case "critical":
			s.Critical++
		case "high":
			s.High++
		case "medium":
			s.Medium++
		case "low":
			s.Low++
		default:
			s.Info++
		}
	}
	return s
}

func printContainerScanResult(r containerScanResult) {
	fmt.Printf("🐳 Container Scan Results — %s via %s\n", r.Target, r.Runtime)
	fmt.Printf("   Kind: %s | Scanners: %s\n", r.Kind, strings.Join(r.Scanners, ", "))
	fmt.Printf("   Severity: Critical %d | High %d | Medium %d | Low %d | Info %d | Total %d\n",
		r.Summary.Critical, r.Summary.High, r.Summary.Medium, r.Summary.Low, r.Summary.Info, r.Summary.Total)

	if len(r.Findings) == 0 {
		fmt.Println("\n   ✅ No container security findings detected")
		return
	}

	fmt.Printf("\n   🚨 Findings (%d)\n", len(r.Findings))
	fmt.Println(strings.Repeat("-", 60))

	ordered := append([]SecurityFinding(nil), r.Findings...)
	orderContainerFindingsBySeverity(ordered)
	for _, f := range ordered {
		fmt.Printf("\n   [%s] %s — %s\n", strings.ToUpper(f.Severity), f.RuleID, f.Tool)
		if f.File != "" {
			fmt.Printf("   File: %s:%d\n", f.File, f.Line)
		}
		fmt.Printf("   %s\n", f.Description)
		if f.Remediation != "" {
			fmt.Printf("   Remediation: %s\n", f.Remediation)
		}
	}
	fmt.Println()
}

func orderContainerFindingsBySeverity(findings []SecurityFinding) {
	severityRank := map[string]int{
		"critical": 0, "high": 1, "medium": 2, "low": 3,
	}
	for i := range findings {
		findings[i].Severity = strings.ToLower(findings[i].Severity)
	}
	for i := 0; i < len(findings); i++ {
		for j := i + 1; j < len(findings); j++ {
			ri := severityRank[findings[i].Severity]
			rj := severityRank[findings[j].Severity]
			if ri > rj || (ri == rj && findings[i].RuleID > findings[j].RuleID) {
				findings[i], findings[j] = findings[j], findings[i]
			}
		}
	}
}

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

// ─────────────────────────────────────────────────────────────────────────────
// SCA scan
// ─────────────────────────────────────────────────────────────────────────────

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

// ─────────────────────────────────────────────────────────────────────────────
// SBOM scan
// ─────────────────────────────────────────────────────────────────────────────

// sbomCacheDir can be overridden by tests to avoid relying on the user's real cache.
var sbomCacheDir = os.UserCacheDir

// sbomParseGoMod is swappable for tests; defaults to the vendored SCA parser.
var sbomParseGoMod = sca.ParseGoMod

// sbomProjectDetectors maps source filenames to their ecosystem type.
// It is used to pick a dependency parser and to report the SBOM source type.
var sbomProjectDetectors = []struct {
	file    string
	ecotype string
	parser  func(string) ([]sbomDep, error)
}{
	{"go.mod", "go", parseGoModDeps},
	{"package.json", "npm", parsePackageJSONDeps},
	{"requirements.txt", "pypi", parseRequirementsTxtDeps},
}

// sbomDep is a minimal dependency shape understood by the SBOM generator.
type sbomDep struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
	PURL    string `json:"purl,omitempty"`
}

// NewSecurityScanSbomCmd returns the `sin-code security scan sbom` subcommand.
func NewSecurityScanSbomCmd() *cobra.Command {
	var (
		format  string
		noBuild bool
		timeout int
		name    string
	)

	cmd := &cobra.Command{
		Use:   "sbom [path]",
		Short: "Generate an SBOM for the project using the vendored generator",
		Long: `sbom generates a Software Bill of Materials (SBOM) for the project at the given path.

It auto-detects the project manifest (go.mod, package.json, requirements.txt),
runs the vendored SIN-Code-SBOM-Generator-Go, and emits SPDX 2.3 or CycloneDX 1.5 JSON.

The generator binary is located in the following order:
  1. $SIN_SBOM_BIN
  2. A binary named "sin-sbom-go" on PATH
  3. The vendored SIN-Code-SBOM-Generator-Go module (built into the user cache if needed)

Use --no-build to fail fast instead of compiling the vendored generator.`,
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

			bin, err := sbomBinLocator(abs, noBuild)
			if err != nil {
				return err
			}

			out, err := runSBOMGenerator(abs, bin, format, name, timeout)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "spdx-json", "Output format: spdx-json, cyclonedx-json")
	cmd.Flags().BoolVar(&noBuild, "no-build", false, "Do not build the vendored generator if missing")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "Timeout in seconds")
	cmd.Flags().StringVarP(&name, "name", "n", "", "Document name for the SBOM (defaults to the project directory name)")

	return cmd
}

// sbomBinLocator is overridable for tests so they can supply a fake generator
// without touching PATH or the filesystem.
var sbomBinLocator = func(scanPath string, noBuild bool) (string, error) {
	return findSBOMGeneratorBinary(scanPath, noBuild)
}

// findSBOMGeneratorBinary resolves the sin-sbom-go binary, building it from the
// vendored module if necessary and allowed.
func findSBOMGeneratorBinary(scanPath string, noBuild bool) (string, error) {
	if env := os.Getenv("SIN_SBOM_BIN"); env != "" {
		if fileExists(env) {
			return env, nil
		}
		return "", fmt.Errorf("SIN_SBOM_BIN points to missing file: %s", env)
	}

	if p, err := exec.LookPath("sin-sbom-go"); err == nil {
		return p, nil
	}

	candidates := []string{scanPath}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	if ex, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Dir(ex))
	}

	for _, start := range candidates {
		moduleDir := findSBOMModuleRoot(start)
		if moduleDir == "" {
			continue
		}
		bin := cachedSBOMBinaryPath()
		if bin == "" {
			return "", fmt.Errorf("cannot determine cache directory for SBOM generator")
		}
		if fileExists(bin) {
			return bin, nil
		}
		if noBuild {
			return "", fmt.Errorf("SBOM generator not found and --no-build prevents build")
		}
		return buildSBOMGenerator(moduleDir)
	}

	return "", fmt.Errorf("SBOM generator not found; install sin-sbom-go or set SIN_SBOM_BIN")
}

// findSBOMModuleRoot walks upward from start looking for a directory that
// contains SIN-Code-SBOM-Generator-Go/go.mod.
func findSBOMModuleRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(abs, "SIN-Code-SBOM-Generator-Go")
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

// cachedSBOMBinaryPath returns the path where the built generator binary is cached.
func cachedSBOMBinaryPath() string {
	dir, err := sbomCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "sin-code", "sin-sbom-go"+binaryExt())
}

// buildSBOMGenerator compiles the vendored generator into the user cache.
func buildSBOMGenerator(moduleDir string) (string, error) {
	out := cachedSBOMBinaryPath()
	if out == "" {
		return "", fmt.Errorf("cannot determine cache directory")
	}
	cacheDir := filepath.Dir(out)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create SBOM generator cache dir: %w", err)
	}

	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("go not found in PATH; cannot build SBOM generator")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Remove any stale binary so a failed build is not masked by an old file.
	_ = os.Remove(out)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/sbom")
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildOut, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build SBOM generator in %s: %w\n%s", moduleDir, err, string(buildOut))
	}
	return out, nil
}

// runSBOMGenerator collects dependencies from path, invokes the generator, and
// returns the produced SBOM bytes for the requested format.
func runSBOMGenerator(path, binary, format, documentName string, timeout int) ([]byte, error) {
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	deps, sourceType, err := collectSBOMDependencies(path)
	if err != nil {
		return nil, fmt.Errorf("collect dependencies: %w", err)
	}

	depsFile, err := writeSBOMDepsFile(path, deps)
	if err != nil {
		return nil, fmt.Errorf("write dependencies file: %w", err)
	}
	defer os.Remove(depsFile)

	if documentName == "" {
		documentName = filepath.Base(path)
	}

	outFile, err := os.CreateTemp("", "sin-sbom-output-*.json")
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}
	_ = outFile.Close()
	defer os.Remove(outFile.Name())

	var outputFlag string
	switch format {
	case "spdx-json":
		outputFlag = "-output-spdx"
	case "cyclonedx-json":
		outputFlag = "-output-cyclonedx"
	default:
		return nil, fmt.Errorf("unsupported format %q; use spdx-json or cyclonedx-json", format)
	}

	args := []string{
		"-deps", depsFile,
		"-name", documentName,
		outputFlag, outFile.Name(),
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = os.Environ()
	combined, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("SBOM generator failed: %w\n%s", err, string(combined))
	}

	out, err := os.ReadFile(outFile.Name())
	if err != nil {
		return nil, fmt.Errorf("read generated SBOM: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("SBOM generator produced empty output (stderr: %s)", string(combined))
	}
	_ = sourceType // kept for future human-readable output; not used in JSON mode
	return out, nil
}

// collectSBOMDependencies inspects the project manifest and returns a list of
// dependencies plus a source-type hint.
func collectSBOMDependencies(path string) ([]sbomDep, string, error) {
	for _, d := range sbomProjectDetectors {
		if fileExists(filepath.Join(path, d.file)) {
			deps, err := d.parser(path)
			if err != nil {
				return nil, "", fmt.Errorf("parse %s: %w", d.file, err)
			}
			return deps, d.ecotype, nil
		}
	}
	// No recognised manifest: return an empty dependency list. The generator
	// will still produce an SBOM document with zero packages.
	return []sbomDep{}, "generic", nil
}

// parseGoModDeps uses the vendored SCA parser to read go.mod.
func parseGoModDeps(path string) ([]sbomDep, error) {
	pkgs, err := sbomParseGoMod(path)
	if err != nil {
		return nil, err
	}
	deps := make([]sbomDep, 0, len(pkgs))
	for _, p := range pkgs {
		version := p.Version
		if version == "" {
			version = "unknown"
		}
		deps = append(deps, sbomDep{
			Name:    p.Name,
			Version: version,
			Type:    "library",
			PURL:    fmt.Sprintf("pkg:golang/%s@%s", p.Name, version),
		})
	}
	return deps, nil
}

// sbomPackageJSON mirrors the subset of package.json we need to parse.
type sbomPackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// parsePackageJSONDeps reads direct and dev dependencies from package.json.
func parsePackageJSONDeps(path string) ([]sbomDep, error) {
	data, err := os.ReadFile(filepath.Join(path, "package.json"))
	if err != nil {
		return nil, err
	}
	var pj sbomPackageJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	deps := make([]sbomDep, 0)
	add := func(name, version string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		cleanVersion := strings.TrimPrefix(version, "^")
		cleanVersion = strings.TrimPrefix(cleanVersion, "~")
		deps = append(deps, sbomDep{
			Name:    name,
			Version: cleanVersion,
			Type:    "library",
			PURL:    fmt.Sprintf("pkg:npm/%s@%s", name, cleanVersion),
		})
	}
	for n, v := range pj.Dependencies {
		add(n, v)
	}
	for n, v := range pj.DevDependencies {
		add(n, v)
	}
	return deps, nil
}

// requirementsLine matches lines like "requests==2.31.0" or "flask>=2.0.0".
var requirementsLine = regexp.MustCompile(`^\s*([A-Za-z0-9_.\-]+)\s*([=<>!~]+)\s*([^\s;#]+)`)

// parseRequirementsTxtDeps parses version-pinned requirements.txt lines.
func parseRequirementsTxtDeps(path string) ([]sbomDep, error) {
	data, err := os.ReadFile(filepath.Join(path, "requirements.txt"))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	deps := make([]sbomDep, 0, len(lines))
	seen := make(map[string]bool)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := requirementsLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		version := m[3]
		if seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, sbomDep{
			Name:    name,
			Version: version,
			Type:    "library",
			PURL:    fmt.Sprintf("pkg:pypi/%s@%s", name, version),
		})
	}
	return deps, nil
}

// writeSBOMDepsFile writes a temporary deps.json file for the SBOM generator.
func writeSBOMDepsFile(path string, deps []sbomDep) (string, error) {
	f, err := os.CreateTemp("", "sin-sbom-deps-*.json")
	if err != nil {
		return "", err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(deps); err != nil {
		return "", err
	}
	return f.Name(), nil
}
