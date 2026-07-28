// SPDX-License-Identifier: MIT
// Purpose: coverage scanner for the SIN-Code Coverage-Drohne.
// Runs `go test` with coverage, parses the output, and exposes a structured
// report that can be used by CI gates or test-generation agents.
// Docs: cmd/sin-code/internal/coverdrohne/coverdrohne.doc.md
package coverdrohne

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PackageCoverage holds the coverage result for one package.
//
// mkdirTemp is swappable for tests that exercise the temp-dir error path.
var mkdirTemp = os.MkdirTemp

// PackageCoverage holds the coverage result for one package.
type PackageCoverage struct {
	ImportPath string  `json:"import_path"`
	Coverage   float64 `json:"coverage"`
	Statements int     `json:"statements"`
	Covered    int     `json:"covered"`
}

// Scanner configures how coverage is collected.
type Scanner struct {
	// GoTest is the go binary to use (default: "go").
	GoTest string
	// Root is the module root to scan from (default: current directory).
	Root string
	// Packages is the package pattern passed to `go test` (default: "./cmd/sin-code/...").
	Packages string
	// Verbose prints the raw go test output to stderr.
	Verbose bool
	// runGoTest is the test hook; tests override it to avoid a real `go test` run.
	runGoTest func(dir, packages, coverprofile string) ([]byte, error)
}

// NewScanner returns a scanner with sane defaults.
func NewScanner() *Scanner {
	return &Scanner{
		GoTest:   "go",
		Packages: "./cmd/sin-code/...",
	}
}

// Scan runs `go test -cover` for all configured packages and returns a sorted
// list of coverage results. The returned slice is sorted by coverage ascending.
func (s *Scanner) Scan() ([]PackageCoverage, error) {
	goTest := s.GoTest
	if goTest == "" {
		goTest = "go"
	}
	root := s.Root
	if root == "" {
		root = "."
	}
	packages := s.Packages
	if packages == "" {
		packages = "./cmd/sin-code/..."
	}

	run := s.runGoTest
	if run == nil {
		run = defaultRunGoTest
	}

	// Use a temporary coverprofile for detailed gap analysis later.
	tmpDir, err := mkdirTemp("", "sin-cover-drohne-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	coverprofile := filepath.Join(tmpDir, "coverage.out")

	out, err := run(root, packages, coverprofile)
	if err != nil {
		return nil, fmt.Errorf("go test failed: %w\n%s", err, string(out))
	}
	if s.Verbose {
		if _, writeErr := os.Stderr.Write(out); writeErr != nil {
			return nil, fmt.Errorf("write verbose coverage output: %w", writeErr)
		}
	}

	results, err := parseGoTestCoverageOutput(string(out))
	if err != nil {
		return nil, err
	}

	return results, nil
}

var coverageLineRe = regexp.MustCompile(`^ok\s+\S+\s+\S+\s+coverage:\s+([0-9.]+)%\s+of\s+statements`)

func parseGoTestCoverageOutput(output string) ([]PackageCoverage, error) {
	var results []PackageCoverage
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		m := coverageLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// Package path is the second token after "ok".
		fields := strings.Fields(line)
		pkg := fields[1]
		pct, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return nil, fmt.Errorf("parse coverage percentage for %s: %w", pkg, err)
		}
		results = append(results, PackageCoverage{
			ImportPath: pkg,
			Coverage:   pct,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool { return results[i].Coverage < results[j].Coverage })
	return results, nil
}

func defaultRunGoTest(dir, packages, coverprofile string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// #nosec G204 -- the executable and flags are fixed; packages is passed as
	// one argv element to `go test` and never interpreted by a shell.
	cmd := exec.CommandContext(ctx, "go", "test", packages, "-count=1", "-p=1", "-coverprofile="+coverprofile)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("go test timed out after 5m: %w", ctx.Err())
	}
	return out, err
}
