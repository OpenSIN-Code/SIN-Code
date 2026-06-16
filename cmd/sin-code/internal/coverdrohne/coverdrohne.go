// SPDX-License-Identifier: MIT
// Purpose: `sin-code cover` command implementation — scan coverage, check gates,
// and prepare test-generation requests.
package coverdrohne

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// mkdirTempCmd is the temp-dir hook used by command helpers.
var mkdirTempCmd = os.MkdirTemp

// writeFileHook is swappable for tests that exercise the --out write path.
var writeFileHook = os.WriteFile

// runGoTestHook is swappable for tests that exercise the go-test error paths.
var runGoTestHook = defaultRunGoTest

// mkdirAllHook is swappable for tests that exercise the hook install error path.
var mkdirAllHook = os.MkdirAll

// writeFileModeHook is swappable for tests that exercise the hook install write error path.
var writeFileModeHook = func(name string, data []byte, perm os.FileMode) error { return os.WriteFile(name, data, perm) }

// scanWithProfileHook is swappable for tests that exercise the scanWithProfile error path.
var scanWithProfileHook = scanWithProfile

// jsonMarshalIndentHook is swappable for tests that exercise the JSON marshal error path.
var jsonMarshalIndentHook = json.MarshalIndent

// NewCommand returns the `sin-code cover` cobra command.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cover",
		Short: "Coverage scanner and test-generation coordinator",
		Long: `sin-code cover scans the Go coverage of all SIN-Code packages
and reports which ones are below the configured threshold.

  sin-code cover scan                  # text table of package coverage
  sin-code cover scan --json           # JSON output
  sin-code cover check --min 100       # exit 1 if any package below 100%
  sin-code cover gaps --package <pkg>  # uncovered functions/blocks for a package
  sin-code cover generate --package <pkg> --out req.json  # AI test-gen request
  sin-code cover hook                  # print a git pre-commit coverage gate
  sin-code cover hook --install        # install .git/hooks/pre-commit`,
	}
	cmd.AddCommand(
		newScanCmd(),
		newCheckCmd(),
		newGapsCmd(),
		newGenerateCmd(),
		newHookCmd(),
	)
	return cmd
}

func newScanCmd() *cobra.Command {
	var jsonOut bool
	var packages string
	var root string
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan package coverage and print a report",
		RunE: func(cmd *cobra.Command, args []string) error {
			scanner := NewScanner()
			scanner.Root = root
			scanner.Packages = packages
			results, err := scanner.Scan()
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-70s %8s\n", "Package", "Coverage")
			fmt.Fprintf(w, "%s\n", strings.Repeat("-", 80))
			for _, r := range results {
				fmt.Fprintf(w, "%-70s %7.1f%%\n", r.ImportPath, r.Coverage)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON report")
	cmd.Flags().StringVar(&packages, "packages", "./cmd/sin-code/...", "package pattern")
	cmd.Flags().StringVar(&root, "root", ".", "module root directory")
	return cmd
}

func newCheckCmd() *cobra.Command {
	var min float64
	var packages string
	var jsonOut bool
	var root string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Fail if any package is below the coverage threshold",
		RunE: func(cmd *cobra.Command, args []string) error {
			scanner := NewScanner()
			scanner.Root = root
			scanner.Packages = packages
			results, err := scanner.Scan()
			if err != nil {
				return err
			}
			var failed []PackageCoverage
			for _, r := range results {
				if r.Coverage < min {
					failed = append(failed, r)
				}
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"passed": len(failed) == 0,
					"min":    min,
					"failed": failed,
				})
			}
			if len(failed) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ all packages meet %.1f%% coverage\n", min)
				return nil
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "✗ %d package(s) below %.1f%% coverage:\n", len(failed), min)
			for _, r := range failed {
				fmt.Fprintf(w, "  %s %.1f%%\n", r.ImportPath, r.Coverage)
			}
			return fmt.Errorf("coverage gate failed")
		},
	}
	cmd.Flags().Float64Var(&min, "min", 100, "minimum coverage percentage")
	cmd.Flags().StringVar(&packages, "packages", "./cmd/sin-code/...", "package pattern")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON report")
	cmd.Flags().StringVar(&root, "root", ".", "module root directory")
	return cmd
}

func newGapsCmd() *cobra.Command {
	var pkg string
	var jsonOut bool
	var packages string
	var root string
	cmd := &cobra.Command{
		Use:   "gaps",
		Short: "Show uncovered blocks for a package",
		RunE: func(cmd *cobra.Command, args []string) error {
			coverprofile, err := runCoverageProfile(root, packages)
			if err != nil {
				return err
			}
			defer os.RemoveAll(filepath.Dir(coverprofile))
			gaps, err := Gaps(coverprofile, root)
			if err != nil {
				return err
			}
			if pkg != "" {
				gaps = filterGapsByPackage(gaps, pkg)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(gaps)
			}
			w := cmd.OutOrStdout()
			for _, g := range gaps {
				fmt.Fprintf(w, "%s\n", g.File)
				for _, b := range g.Blocks {
					fmt.Fprintf(w, "  %s:%d-%d (%d stmts)\n", b.FuncName, b.StartLine, b.EndLine, b.NumStmts)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&pkg, "package", "", "filter by package import path substring")
	cmd.Flags().StringVar(&packages, "packages", "./cmd/sin-code/...", "package pattern")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON report")
	cmd.Flags().StringVar(&root, "root", ".", "module root directory")
	return cmd
}

func newGenerateCmd() *cobra.Command {
	var pkg string
	var out string
	var packages string
	var root string
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Write a test-generation request JSON for a package",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pkg == "" {
				return fmt.Errorf("--package is required")
			}
			coverprofile, err := runCoverageProfile(root, packages)
			if err != nil {
				return err
			}
			defer os.RemoveAll(filepath.Dir(coverprofile))

			results, err := scanWithProfileHook(root, packages, coverprofile)
			if err != nil {
				return err
			}
			var target *PackageCoverage
			for i := range results {
				if strings.Contains(results[i].ImportPath, pkg) {
					target = &results[i]
					break
				}
			}
			if target == nil {
				return fmt.Errorf("package %q not found in coverage scan", pkg)
			}

			gaps, err := Gaps(coverprofile, root)
			if err != nil {
				return err
			}
			gaps = filterGapsByPackage(gaps, pkg)

			req := map[string]any{
				"package":  target.ImportPath,
				"coverage": target.Coverage,
				"gaps":     gaps,
				"prompt": fmt.Sprintf("Add Go tests to bring %s from %.1f%% to 100%% coverage. "+
					"Target the uncovered functions/blocks listed in gaps.", target.ImportPath, target.Coverage),
			}
			data, err := jsonMarshalIndentHook(req, "", "  ")
			if err != nil {
				return err
			}
			if out == "" {
				_, err = cmd.OutOrStdout().Write(data)
				_, _ = cmd.OutOrStdout().Write([]byte("\n"))
				return err
			}
			return writeFileHook(out, data, 0o644)
		},
	}
	cmd.Flags().StringVar(&pkg, "package", "", "package import path substring")
	cmd.Flags().StringVar(&out, "out", "", "output file (default: stdout)")
	cmd.Flags().StringVar(&packages, "packages", "./cmd/sin-code/...", "package pattern")
	cmd.Flags().StringVar(&root, "root", ".", "module root directory")
	return cmd
}

func newHookCmd() *cobra.Command {
	var min float64
	var install bool
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Print or install a git pre-commit hook that runs the coverage gate",
		RunE: func(cmd *cobra.Command, args []string) error {
			script := preCommitHookScript(min)
			if !install {
				_, err := cmd.OutOrStdout().Write([]byte(script))
				return err
			}
			dir := ".git/hooks"
			if err := mkdirAllHook(dir, 0o755); err != nil {
				return err
			}
			path := filepath.Join(dir, "pre-commit")
			if err := writeFileModeHook(path, []byte(script), 0o755); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed %s\n", path)
			return nil
		},
	}
	cmd.Flags().Float64Var(&min, "min", 100, "minimum coverage percentage")
	cmd.Flags().BoolVar(&install, "install", false, "write the hook to .git/hooks/pre-commit")
	return cmd
}

func preCommitHookScript(min float64) string {
	return fmt.Sprintf(`#!/bin/sh
# Generated by sin-code cover hook. Runs the coverage gate before every commit.
set -e
sin-code cover check --min %.1f
`, min)
}

func runCoverageProfile(root, packages string) (string, error) {
	if packages == "" {
		packages = "./cmd/sin-code/..."
	}
	tmpDir, err := mkdirTempCmd("", "sin-cover-drohne-*")
	if err != nil {
		return "", err
	}
	coverprofile := filepath.Join(tmpDir, "coverage.out")
	out, err := runGoTestHook(root, packages, coverprofile)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("go test failed: %w\n%s", err, string(out))
	}
	return coverprofile, nil
}

func scanWithProfile(root, packages, coverprofile string) ([]PackageCoverage, error) {
	out, err := runGoTestHook(root, packages, coverprofile)
	if err != nil {
		return nil, fmt.Errorf("go test failed: %w\n%s", err, string(out))
	}
	return parseGoTestCoverageOutput(string(out))
}

func filterGapsByPackage(gaps []Gap, pkg string) []Gap {
	var out []Gap
	for _, g := range gaps {
		if strings.Contains(g.File, pkg) {
			out = append(out, g)
		}
	}
	return out
}
