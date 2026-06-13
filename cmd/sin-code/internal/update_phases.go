// SPDX-License-Identifier: MIT
// Purpose: Update phases — Python (pipx), Go (rebuild), Skills (pipx metadata).
// Docs: update_phases.doc.md
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var AllPythonPackages = []string{
	"sin-code-bundle",
	"sin-codocs", "sin-websearch", "sin-scheduler", "sin-goal-mode",
	"sin-frontend-design", "sin-doc-coauthoring", "sin-slash", "sin-grill-me",
	"sin-marketplace", "sin-mcp-server-builder", "sin-honcho-rollback",
	"sin-context-bridge", "sin-browser-tools", "sin-git-workflow",
	"sin-preview", "sin-image-generation", "sin-new-vercel-token-in-pool",
	"sin-adhd", "sin-ceo-audit",
}

type GoTool struct {
	Name    string
	Repo    string
	CmdPath string
}

var AllGoTools = []GoTool{
	{Name: "discover", Repo: "SIN-Code-Discover-Tool", CmdPath: "cmd/discover"},
	{Name: "execute", Repo: "SIN-Code-Execute-Tool", CmdPath: "cmd/execute"},
	{Name: "map", Repo: "SIN-Code-Map-Tool", CmdPath: "cmd/map"},
	{Name: "grasp", Repo: "SIN-Code-Grasp-Tool", CmdPath: "cmd/grasp"},
	{Name: "scout", Repo: "SIN-Code-Scout-Tool", CmdPath: "cmd/scout"},
	{Name: "harvest", Repo: "SIN-Code-Harvest-Tool", CmdPath: "cmd/harvest"},
	{Name: "orchestrate", Repo: "SIN-Code-Orchestrate-Tool", CmdPath: "cmd/orchestrate"},
}

type PhaseResult struct {
	Name    string
	Updated int
	Skipped int
	Failed  int
	Errors  []string
}

var execPipx = func(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "pipx", args...)
}

var execGo = func(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "go", args...)
}

var execGit = func(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", args...)
}

func RunPythonPhase(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
	res := &PhaseResult{Name: "python"}
	if opts.CheckOnly || opts.DryRun {
		return enumeratePipx(ctx, res, opts)
	}
	for _, pkg := range AllPythonPackages {
		if err := runPipxUpgrade(ctx, pkg); err != nil {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", pkg, err))
			continue
		}
		res.Updated++
	}
	if gsdList, err := listGsdFamily(ctx); err == nil {
		for _, pkg := range gsdList {
			if err := runPipxUpgrade(ctx, pkg); err != nil {
				res.Failed++
				res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", pkg, err))
				continue
			}
			res.Updated++
		}
	}
	return res, nil
}

func RunGoPhase(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
	res := &PhaseResult{Name: "go"}
	reposDir := os.Getenv("SIN_CODE_REPOS_DIR")
	if reposDir == "" {
		reposDir = filepath.Join(homeDirOrEmpty(), "dev")
	}
	binDir := binDirPath()
	for _, tool := range AllGoTools {
		repoPath := filepath.Join(reposDir, tool.Repo)
		if _, err := os.Stat(repoPath); err != nil {
			res.Skipped++
			fmt.Fprintf(os.Stderr, "[warn] %s: repo not found at %s -- skipping\n", tool.Name, repoPath)
			continue
		}
		version, err := gitDescribeVersion(ctx, repoPath)
		if err != nil {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Sprintf("%s: git describe: %v", tool.Name, err))
			continue
		}
		if opts.CheckOnly || opts.DryRun {
			res.Skipped++
			continue
		}
		binaryPath := filepath.Join(binDir, tool.Name)
		ldflags := fmt.Sprintf("-X main.Version=%s", version)
		cmd := execGo(ctx, "build",
			"-ldflags", ldflags,
			"-o", binaryPath,
			"./"+tool.CmdPath,
		)
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Sprintf("%s: build failed: %v\n%s", tool.Name, err, out))
			continue
		}
		if err := verifyBinaryVersion(ctx, binaryPath, version); err != nil {
			res.Failed++
			res.Errors = append(res.Errors, fmt.Sprintf("%s: post-build verify: %v", tool.Name, err))
			continue
		}
		res.Updated++
	}
	return res, nil
}

func RunSkillsPhase(ctx context.Context, opts UpdateOptions) (*PhaseResult, error) {
	return RunPythonPhase(ctx, opts)
}

func runPipxUpgrade(ctx context.Context, pkg string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := execPipx(ctx, "upgrade", pkg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%s", err, string(out))
	}
	return nil
}

func enumeratePipx(ctx context.Context, res *PhaseResult, opts UpdateOptions) (*PhaseResult, error) {
	for _, pkg := range AllPythonPackages {
		fmt.Printf("  [check] pipx: %s\n", pkg)
		res.Skipped++
	}
	return res, nil
}

func listGsdFamily(ctx context.Context) ([]string, error) {
	cmd := execPipx(ctx, "list", "--json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	var result struct {
		Venvs map[string]interface{} `json:"venvs"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	var gsdPkgs []string
	for name := range result.Venvs {
		if strings.HasPrefix(name, "sin-gsd-") {
			gsdPkgs = append(gsdPkgs, name)
		}
	}
	return gsdPkgs, nil
}

func gitDescribeVersion(ctx context.Context, repoPath string) (string, error) {
	cmd := execGit(ctx, "describe", "--tags", "--dirty")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func verifyBinaryVersion(ctx context.Context, binaryPath, expectedVersion string) error {
	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), expectedVersion) {
		return fmt.Errorf("version mismatch: expected %q in output, got %q", expectedVersion, strings.TrimSpace(string(out)))
	}
	return nil
}

func homeDirOrEmpty() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func binDirPath() string {
	binDir := os.Getenv("SIN_CODE_BIN_DIR")
	if binDir == "" {
		binDir = filepath.Join(homeDirOrEmpty(), ".local", "bin")
	}
	return binDir
}

func runDoctorNonFatal(ctx context.Context) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "doctor")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func printPhaseSummary(results []*PhaseResult) {
	fmt.Println()
	fmt.Println("-- Update Summary --")
	for _, r := range results {
		fmt.Printf("  %s: %d updated, %d skipped, %d failed\n", r.Name, r.Updated, r.Skipped, r.Failed)
		for _, e := range r.Errors {
			fmt.Fprintf(os.Stderr, "  [error] %s\n", e)
		}
	}
}

func runCheck(ctx context.Context, opts UpdateOptions) error {
	fmt.Println("-- Update Check --")
	fmt.Println()

	offline := os.Getenv("SIN_CODE_OFFLINE")
	if offline != "" || os.Getenv("NO_UPDATE_CHECK") != "" {
		fmt.Println("Offline mode -- skipping network checks.")
		return nil
	}

	res, err := RunPythonPhase(ctx, opts)
	if err != nil {
		return err
	}
	printPhaseSummary([]*PhaseResult{res})

	goRes, err := RunGoPhase(ctx, opts)
	if err != nil {
		return err
	}
	printPhaseSummary([]*PhaseResult{goRes})

	return nil
}

func currentPlatformString() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}
