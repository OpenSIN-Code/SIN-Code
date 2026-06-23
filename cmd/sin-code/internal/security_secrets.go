// SPDX-License-Identifier: MIT
// Purpose: `sin-code security scan secrets` — vendored secrets scanner
// integration. Locates the `sin-secrets` binary (or builds it from the
// vendored SIN-Code-Secrets-Scanner module), runs it with JSON output,
// parses the result, and prints findings in a friendly format.
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
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// ─── Data models (mirror of SIN-Code-Secrets-Scanner/pkg/models) ─────────

type secretsScanFinding struct {
	RuleID      string  `json:"rule_id"`
	RuleName    string  `json:"rule_name"`
	Severity    string  `json:"severity"`
	SecretType  string  `json:"secret_type"`
	File        string  `json:"file"`
	Line        int     `json:"line"`
	Column      int     `json:"column"`
	Match       string  `json:"match"`
	Context     string  `json:"context"`
	Remediation string  `json:"remediation"`
	Confidence  string  `json:"confidence"`
	Entropy     float64 `json:"entropy"`
	IsVerified  bool    `json:"is_verified"`
}

type secretsScanSummary struct {
	Critical     int            `json:"critical"`
	High         int            `json:"high"`
	Medium       int            `json:"medium"`
	Low          int            `json:"low"`
	FilesScanned int            `json:"files_scanned"`
	SecretsFound int            `json:"secrets_found"`
	ByType       map[string]int `json:"by_type"`
	ByFile       map[string]int `json:"by_file"`
}

type secretsScanResult struct {
	Path                string               `json:"path"`
	Status              string               `json:"status"`
	Findings            []secretsScanFinding `json:"findings"`
	Summary             secretsScanSummary   `json:"summary"`
	ScanDurationSeconds float64              `json:"scan_duration_seconds"`
	Timestamp           string               `json:"timestamp"`
}

// ─── Command tree ─────────────────────────────────────────────────────────

// NewSecurityScanCmd returns the `sin-code security scan` parent command.
func NewSecurityScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Specialized security scans",
		Long:  `scan runs focused security scanners (e.g. secrets detection) on a project path.`,
	}
	cmd.AddCommand(NewSecurityScanSecretsCmd())
	cmd.AddCommand(NewSecurityScanSastCmd())
	return cmd
}

// NewSecurityScanSecretsCmd returns the `sin-code security scan secrets` subcommand.
func NewSecurityScanSecretsCmd() *cobra.Command {
	var (
		severity  string
		types     string
		exclude   string
		format    string
		strict    bool
		timeout   int
		noEntropy bool
		noBuild   bool
	)

	cmd := &cobra.Command{
		Use:   "secrets [path]",
		Short: "Scan the workspace for leaked secrets and credentials",
		Long: `secrets runs the vendored SIN-Code Secrets Scanner against the given path.

It detects 22+ secret types (API keys, tokens, passwords, private keys,
config files) and prints a concise summary. JSON output is available for CI.

The scanner binary is located in the following order:
  1. $SIN_SECRETS_BIN
  2. A binary named "sin-secrets" on PATH
  3. The vendored SIN-Code-Secrets-Scanner module (built into the user cache if needed)

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

			bin, err := secretsBinLocator(abs, noBuild)
			if err != nil {
				return err
			}

			result, err := runSecretsScanner(bin, abs, severity, types, exclude, timeout, !noEntropy)
			if err != nil {
				return err
			}

			if format == "json" {
				out, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(out))
			} else {
				printSecretsResult(result)
			}

			if strict && result.Summary.SecretsFound > 0 {
				return fmt.Errorf("secrets scan found %d leaked secrets (strict mode)", result.Summary.SecretsFound)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&severity, "severity", "s", "low", "Minimum severity: low, medium, high, critical")
	cmd.Flags().StringVarP(&types, "types", "t", "", "Comma-separated secret types (api-key,token,password,private-key,certificate,config-file)")
	cmd.Flags().StringVarP(&exclude, "exclude", "e", "", "Comma-separated patterns to exclude")
	cmd.Flags().StringVarP(&format, "format", "f", "text", "Output format: text, json")
	cmd.Flags().BoolVar(&strict, "strict", false, "Exit with error if any secrets are found")
	cmd.Flags().IntVar(&timeout, "timeout", 300, "Timeout per scan in seconds")
	cmd.Flags().BoolVar(&noEntropy, "no-entropy", false, "Disable entropy filtering")
	cmd.Flags().BoolVar(&noBuild, "no-build", false, "Do not build the vendored scanner if missing")

	return cmd
}

// secretsBinLocator is overridable for tests so they can supply a fake scanner
// without touching PATH or the filesystem.
var secretsBinLocator = func(scanPath string, noBuild bool) (string, error) {
	return findSecretsScannerBinary(scanPath, noBuild)
}

// findSecretsScannerBinary resolves the sin-secrets binary, building it from the
// vendored module if necessary and allowed.
func findSecretsScannerBinary(scanPath string, noBuild bool) (string, error) {
	if env := os.Getenv("SIN_SECRETS_BIN"); env != "" {
		if fileExists(env) {
			return env, nil
		}
		return "", fmt.Errorf("SIN_SECRETS_BIN points to missing file: %s", env)
	}

	if p, err := exec.LookPath("sin-secrets"); err == nil {
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
		moduleDir := findSecretsModuleRoot(start)
		if moduleDir == "" {
			continue
		}
		bin := cachedSecretsBinaryPath()
		if bin == "" {
			return "", fmt.Errorf("cannot determine cache directory for secrets scanner")
		}
		if fileExists(bin) {
			return bin, nil
		}
		if noBuild {
			return "", fmt.Errorf("secrets scanner not found and --no-build prevents build")
		}
		return buildSecretsScanner(moduleDir)
	}

	return "", fmt.Errorf("secrets scanner not found; install sin-secrets or set SIN_SECRETS_BIN")
}

// findSecretsModuleRoot walks upward from start looking for a directory that
// contains SIN-Code-Secrets-Scanner/go.mod.
func findSecretsModuleRoot(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(abs, "SIN-Code-Secrets-Scanner")
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

// cachedSecretsBinaryPath returns the path where the built scanner binary is cached.
func cachedSecretsBinaryPath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "sin-code", "sin-secrets"+binaryExt())
}

func binaryExt() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// buildSecretsScanner compiles the vendored scanner into the user cache.
func buildSecretsScanner(moduleDir string) (string, error) {
	out := cachedSecretsBinaryPath()
	if out == "" {
		return "", fmt.Errorf("cannot determine cache directory")
	}
	cacheDir := filepath.Dir(out)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create secrets scanner cache dir: %w", err)
	}

	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("go not found in PATH; cannot build secrets scanner")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Remove any stale binary so a failed build is not masked by an old file.
	_ = os.Remove(out)

	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/sin-secrets")
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildOut, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build secrets scanner in %s: %w\n%s", moduleDir, err, string(buildOut))
	}
	return out, nil
}

// runSecretsScanner invokes the scanner binary and parses its JSON output.
func runSecretsScanner(bin, path, severity, types, exclude string, timeout int, checkEntropy bool) (*secretsScanResult, error) {
	if timeout <= 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	args := []string{"scan", path, "--output", "json", "--severity", severity}
	if !checkEntropy {
		args = append(args, "--check-entropy=false")
	}
	if types != "" {
		args = append(args, "--types", types)
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
			return nil, fmt.Errorf("secrets scanner failed: %w\n%s", err, string(out))
		}
		// The scanner exits non-zero when findings are present; keep parsing.
	}

	var result secretsScanResult
	if jerr := json.Unmarshal(out, &result); jerr != nil {
		return nil, fmt.Errorf("parse secrets scanner output: %w\n%s", jerr, string(out))
	}
	return &result, nil
}

// printSecretsResult renders a secrets scan in a human-friendly format.
func printSecretsResult(r *secretsScanResult) {
	fmt.Printf("🔐 Secrets Scan Results — %s\n", r.Path)
	fmt.Printf("   Duration: %.2fs | Status: %s\n", r.ScanDurationSeconds, strings.ToUpper(r.Status))
	fmt.Printf("   Files scanned: %d | Findings: %d\n", r.Summary.FilesScanned, r.Summary.SecretsFound)

	if r.Summary.SecretsFound > 0 {
		fmt.Printf("   Severity: Critical %d | High %d | Medium %d | Low %d\n\n",
			r.Summary.Critical, r.Summary.High, r.Summary.Medium, r.Summary.Low)
		for _, f := range r.Findings {
			fmt.Printf("   [%s] %s — %s (%s)\n", f.Severity, f.RuleID, f.RuleName, f.SecretType)
			fmt.Printf("   File: %s:%d\n", f.File, f.Line)
			fmt.Printf("   Match: %s\n", maskSecuritySecret(f.Match))
			if f.Entropy > 0 {
				fmt.Printf("   Entropy: %.2f\n", f.Entropy)
			}
			fmt.Printf("   Remediation: %s\n", f.Remediation)
			fmt.Println()
		}
	} else {
		fmt.Println("   ✅ No leaked secrets detected")
	}
}

// maskSecuritySecret masks the middle of a secret string so output does not leak it.
func maskSecuritySecret(s string) string {
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}
