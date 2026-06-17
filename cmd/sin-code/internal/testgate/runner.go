// SPDX-License-Identifier: MIT
// Purpose: Quality gate pipeline runner for SIN-Code. Executes build,
// vet, test, staticcheck, gosec, govulncheck with graceful degradation
// when optional tools are missing on PATH.
// Docs: runner.doc.md
package testgate

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Run executes the configured pipeline and returns a structured report.
// The pipeline stops early when a required step fails; optional steps are
// skipped with a warning when the tool is not on PATH.
func Run(ctx context.Context, cfg Config) Report {
	start := time.Now()
	if cfg.CommandRunner == nil {
		cfg.CommandRunner = defaultCommandRunner
	}
	if cfg.LookPath == nil {
		cfg.LookPath = exec.LookPath
	}

	timeout := cfg.effectiveTimeout()
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	steps := cfg.effectiveSteps()
	report := Report{
		Threshold: cfg.CoverageThreshold,
		Steps:     make([]StepReport, 0, len(steps)),
	}

	failed := false
	var coverage string

	for _, kind := range steps {
		if failed {
			report.Steps = append(report.Steps, StepReport{
				Name:    string(kind),
				Status:  "SKIP",
				Skipped: true,
				Error:   "skipped because an earlier step failed",
			})
			continue
		}

		step := runStep(cctx, cfg, kind, coverage)
		report.Steps = append(report.Steps, step)

		if step.Coverage != "" {
			coverage = step.Coverage
			report.Coverage = coverage
		}

		if !step.Skipped && step.Status != "PASS" {
			failed = true
		}
	}

	if cfg.CoverageThreshold > 0 && coverage != "" {
		pct := parseCoveragePercent(coverage)
		if pct < cfg.CoverageThreshold {
			failed = true
			report.Steps = append(report.Steps, StepReport{
				Name:   "coverage-threshold",
				Status: "FAIL",
				Error:  fmt.Sprintf("coverage %.1f%% below threshold %.1f%%", pct, cfg.CoverageThreshold),
			})
		}
	}

	report.Duration = time.Since(start)
	if failed {
		report.Status = "FAIL"
	} else {
		report.Status = "PASS"
	}
	return report
}

func runStep(ctx context.Context, cfg Config, kind StepKind, prevCoverage string) StepReport {
	start := time.Now()
	var name string
	var args []string
	var required bool
	var capturesCoverage bool

	switch kind {
	case StepBuild:
		name, args, required = "go", []string{"build", "./..."}, true
	case StepVet:
		name, args, required = "go", []string{"vet", "./..."}, true
	case StepTest:
		name, args, required = "go", []string{"test", "./...", "-count=1", "-race", "-coverprofile=.sin-code/coverage.out", "-covermode=atomic"}, true
		capturesCoverage = true
	case StepStaticcheck:
		name, args, required = "staticcheck", []string{"./..."}, false
	case StepGosec:
		name, args, required = "gosec", []string{"./..."}, false
	case StepGovulncheck:
		name, args, required = "govulncheck", []string{"./..."}, false
	default:
		return StepReport{Name: string(kind), Status: "FAIL", Error: "unknown step"}
	}

	if !required {
		if _, err := cfg.LookPath(name); err != nil {
			return StepReport{
				Name:    string(kind),
				Status:  "SKIP",
				Skipped: true,
				Error:   fmt.Sprintf("%s not found on PATH", name),
			}
		}
	}

	if kind == StepTest && !cfg.Race {
		args = removeArg(args, "-race")
	}

	out, err := cfg.CommandRunner(ctx, name, args, cfg.Workdir, cfg.effectiveTimeout())
	status := "PASS"
	var errStr string
	if err != nil {
		status = "FAIL"
		errStr = err.Error()
	}

	step := StepReport{
		Name:     string(kind),
		Status:   status,
		Output:   out,
		Error:    errStr,
		Duration: time.Since(start),
	}

	if capturesCoverage && status == "PASS" {
		step.Coverage = extractCoverageLine(out)
		if step.Coverage == "" {
			step.Coverage = prevCoverage
		}
	}

	return step
}

func defaultCommandRunner(ctx context.Context, name string, args []string, dir string, timeout time.Duration) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func removeArg(args []string, target string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a != target {
			out = append(out, a)
		}
	}
	return out
}

func extractCoverageLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "coverage:") && strings.Contains(line, "of statements") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func parseCoveragePercent(s string) float64 {
	if s == "" {
		return 0
	}
	// "coverage: 82.4% of statements"
	parts := strings.Fields(s)
	for _, p := range parts {
		if strings.HasSuffix(p, "%") {
			v, err := strconv.ParseFloat(strings.TrimSuffix(p, "%"), 64)
			if err == nil {
				return v
			}
		}
	}
	return 0
}
