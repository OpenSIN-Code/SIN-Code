// SPDX-License-Identifier: MIT
// Purpose: `sin-code security scan container` — scan a container image or local
// path inside a container runtime. Prefers Apple `container` (https://github.com/apple/container)
// over Docker, mounts the target filesystem and vendored scanner binaries, and
// runs secrets/SAST/SCA scanners inside the container. Runtime detection is
// fully swappable for tests so the command works on machines without either
// runtime installed.
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
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/security/sca"
)

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
