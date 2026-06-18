// SPDX-License-Identifier: MIT
// Purpose: SWE-bench evaluation harness (issue #363). Loads SWE-bench
// JSON instances, runs sin-code against each GitHub issue, applies the
// predicted patch, evaluates it with test_patch, and records pass/fail.
//
// Docs: cmd/sin-code/internal/eval/swebench.doc.md
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SweInstance is one entry from a SWE-bench dataset.
type SweInstance struct {
	InstanceID     string   `json:"instance_id"`
	Repo           string   `json:"repo"`
	BaseCommit     string   `json:"base_commit"`
	Problem        string   `json:"problem_statement"`
	Hints          string   `json:"hints_text,omitempty"`
	FailToPass     []string `json:"FAIL_TO_PASS"`
	PassToPass     []string `json:"PASS_TO_PASS"`
	TestPatch      string   `json:"test_patch"`
	Version        string   `json:"version,omitempty"`
	RepoDir        string   `json:"repo_directory,omitempty"`
}

// SweResult is the outcome for one SWE-bench instance.
type SweResult struct {
	InstanceID  string        `json:"instance_id"`
	Repo        string        `json:"repo"`
	Passed      bool          `json:"passed"`
	Error       string        `json:"error,omitempty"`
	Duration    time.Duration `json:"duration_ns"`
	Turns       int           `json:"turns"`
	Patch       string        `json:"patch,omitempty"`
	TestsPassed int           `json:"tests_passed"`
	TestsTotal  int           `json:"tests_total"`
}

// SweReport is the full SWE-bench evaluation report.
type SweReport struct {
	Dataset      string       `json:"dataset"`
	Total        int          `json:"total"`
	Passed       int          `json:"passed"`
	Failed       int          `json:"failed"`
	PassRate     float64      `json:"pass_rate"`
	TotalDurMS   float64      `json:"total_duration_ms"`
	StartedAt    time.Time    `json:"started_at"`
	FinishedAt   time.Time    `json:"finished_at"`
	Results      []SweResult  `json:"results"`
}

// SweConfig configures the SWE-bench evaluation run.
type SweConfig struct {
	// DatasetPath is the path to a SWE-bench JSON file.
	DatasetPath string
	// OutputPath is where the results JSON is written.
	OutputPath string
	// Workspace is the directory where repos are cloned.
	Workspace string
	// MaxTurns caps agent turns per instance.
	MaxTurns int
	// Timeout per instance.
	Timeout time.Duration
	// SinCodeBin is the path to the sin-code binary.
	SinCodeBin string
	// DryRun skips actual agent runs (for validation).
	DryRun bool
}

// RunSweBench loads a SWE-bench dataset and evaluates sin-code on it.
func RunSweBench(ctx context.Context, cfg SweConfig) (*SweReport, error) {
	if cfg.DatasetPath == "" {
		return nil, fmt.Errorf("swebench: dataset path is required")
	}
	if cfg.OutputPath == "" {
		cfg.OutputPath = "swebench-results.json"
	}
	if cfg.Workspace == "" {
		cfg.Workspace = filepath.Join(os.TempDir(), "sin-code-swebench")
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 100
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Minute
	}
	if cfg.SinCodeBin == "" {
		bin, err := os.Executable()
		if err != nil {
			bin = "sin-code"
		}
		cfg.SinCodeBin = bin
	}

	instances, err := loadSweInstances(cfg.DatasetPath)
	if err != nil {
		return nil, fmt.Errorf("swebench: load instances: %w", err)
	}

	started := time.Now().UTC()
	results := make([]SweResult, 0, len(instances))

	for _, inst := range instances {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		res := SweResult{
			InstanceID: inst.InstanceID,
			Repo:       inst.Repo,
		}

		if cfg.DryRun {
			res.Passed = true
			res.Patch = "(dry run)"
			results = append(results, res)
			continue
		}

		dur, patch, err := evaluateInstance(ctx, cfg, inst)
		res.Duration = dur
		res.Patch = patch
		if err != nil {
			res.Error = err.Error()
			res.Passed = false
		} else {
			res.Passed = true
		}
		results = append(results, res)
	}

	finished := time.Now().UTC()
	report := buildSweReport(cfg.DatasetPath, results, started, finished)
	if err := writeSweReport(cfg.OutputPath, report); err != nil {
		return nil, fmt.Errorf("swebench: write report: %w", err)
	}
	return report, nil
}

func loadSweInstances(path string) ([]SweInstance, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	var instances []SweInstance
	if err := json.Unmarshal(raw, &instances); err != nil {
		var single SweInstance
		if uerr := json.Unmarshal(raw, &single); uerr != nil {
			return nil, fmt.Errorf("parse swebench json: %w (single: %v)", err, uerr)
		}
		instances = []SweInstance{single}
	}
	for i := range instances {
		if instances[i].InstanceID == "" {
			instances[i].InstanceID = fmt.Sprintf("instance-%d", i)
		}
	}
	return instances, nil
}

func evaluateInstance(ctx context.Context, cfg SweConfig, inst SweInstance) (time.Duration, string, error) {
	start := time.Now()

	repoDir := filepath.Join(cfg.Workspace, sanitizeRepo(inst.Repo), inst.InstanceID)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return 0, "", fmt.Errorf("mkdir: %w", err)
	}

	if err := setupRepo(repoDir, inst); err != nil {
		return 0, "", fmt.Errorf("setup repo: %w", err)
	}

	patchFile := filepath.Join(repoDir, "predicted.patch")
	agentCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	patch, err := runAgent(agentCtx, cfg.SinCodeBin, repoDir, inst, patchFile)
	if err != nil {
		return time.Since(start), patch, fmt.Errorf("agent: %w", err)
	}

	if err := applyPatch(repoDir, patch); err != nil {
		return time.Since(start), patch, fmt.Errorf("apply patch: %w", err)
	}

	if err := applyTestPatch(repoDir, inst.TestPatch); err != nil {
		return time.Since(start), patch, fmt.Errorf("apply test patch: %w", err)
	}

	if err := runTests(repoDir, inst); err != nil {
		return time.Since(start), patch, err
	}

	return time.Since(start), patch, nil
}

func setupRepo(repoDir string, inst SweInstance) error {
	gitDir := filepath.Join(repoDir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return nil
	}

	repoURL := fmt.Sprintf("https://github.com/%s.git", inst.Repo)
	cmd := exec.Command("git", "clone", "--depth=1", repoURL, ".")
	cmd.Dir = repoDir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone %s: %w", repoURL, err)
	}

	if inst.BaseCommit != "" {
		checkout := exec.Command("git", "checkout", inst.BaseCommit)
		checkout.Dir = repoDir
		checkout.Stdout = io.Discard
		checkout.Stderr = io.Discard
		if err := checkout.Run(); err != nil {
			return fmt.Errorf("git checkout %s: %w", inst.BaseCommit, err)
		}
	}
	return nil
}

func runAgent(ctx context.Context, bin, repoDir string, inst SweInstance, patchFile string) (string, error) {
	prompt := fmt.Sprintf(`Fix the following issue in the repository at %s.

Issue description:
%s

Hints:
%s

After fixing, create a git diff patch and save it to %s.

Make sure the patch includes only the minimal changes needed to fix the issue.
Do not include test files in the patch.
`, repoDir, inst.Problem, inst.Hints, patchFile)

	cmd := exec.CommandContext(ctx, bin, "-p", prompt)
	cmd.Dir = repoDir
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("sin-code agent: %w", err)
	}

	patch, err := os.ReadFile(patchFile)
	if err != nil {
		if os.IsNotExist(err) {
			diffCmd := exec.Command("git", "diff")
			diffCmd.Dir = repoDir
			out, derr := diffCmd.Output()
			if derr != nil {
				return "", fmt.Errorf("no patch file and git diff failed: %v", derr)
			}
			patch = out
		} else {
			return "", fmt.Errorf("read patch: %w", err)
		}
	}

	return string(patch), nil
}

func applyPatch(repoDir, patch string) error {
	if strings.TrimSpace(patch) == "" {
		return fmt.Errorf("empty patch")
	}
	cmd := exec.Command("git", "apply", "--index")
	cmd.Dir = repoDir
	cmd.Stdin = strings.NewReader(patch)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func applyTestPatch(repoDir, testPatch string) error {
	if strings.TrimSpace(testPatch) == "" {
		return nil
	}
	cmd := exec.Command("git", "apply", "--index")
	cmd.Dir = repoDir
	cmd.Stdin = strings.NewReader(testPatch)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func runTests(repoDir string, inst SweInstance) error {
	targets := append(inst.FailToPass, inst.PassToPass...)
	if len(targets) == 0 {
		return nil
	}

	var failures []string
	for _, target := range targets {
		cmd := exec.Command("python", "-m", "pytest", target, "-x", "--timeout=120", "-q")
		cmd.Dir = repoDir
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		if err := cmd.Run(); err != nil {
			failures = append(failures, target)
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("test failures: %s", strings.Join(failures, ", "))
	}
	return nil
}

func buildSweReport(dataset string, results []SweResult, started, finished time.Time) *SweReport {
	passed := 0
	var totalDur time.Duration
	for _, r := range results {
		if r.Passed {
			passed++
		}
		totalDur += r.Duration
	}
	total := len(results)
	rate := 0.0
	if total > 0 {
		rate = float64(passed) / float64(total)
	}
	return &SweReport{
		Dataset:    dataset,
		Total:      total,
		Passed:     passed,
		Failed:     total - passed,
		PassRate:   rate,
		TotalDurMS: float64(totalDur.Microseconds()) / 1000.0,
		StartedAt:  started,
		FinishedAt: finished,
		Results:    results,
	}
}

func writeSweReport(path string, report *SweReport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// SwePrintSummary prints a human-readable summary of the SWE-bench report.
func SwePrintSummary(w io.Writer, r *SweReport) {
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, "SWE-bench Results: %s\n", r.Dataset)
	fmt.Fprintf(w, "  Total:  %d\n", r.Total)
	fmt.Fprintf(w, "  Passed: %d\n", r.Passed)
	fmt.Fprintf(w, "  Failed: %d\n", r.Failed)
	fmt.Fprintf(w, "  Pass Rate: %.2f%%\n", r.PassRate*100)
	fmt.Fprintf(w, "  Duration: %.0f ms\n", r.TotalDurMS)
	fmt.Fprintln(w)
	if r.Total > 0 && len(r.Results) <= 50 {
		fmt.Fprintf(w, "%-40s %s\n", "Instance", "Result")
		fmt.Fprintf(w, "%-40s %s\n", strings.Repeat("-", 40), strings.Repeat("-", 8))
		for _, res := range r.Results {
			status := "PASS"
			if !res.Passed {
				status = "FAIL"
			}
			label := res.InstanceID
			if len(label) > 39 {
				label = label[:39]
			}
			fmt.Fprintf(w, "%-40s %s\n", label, status)
		}
	}
}

func sanitizeRepo(repo string) string {
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(repo)
}

var _ = sort.Strings
