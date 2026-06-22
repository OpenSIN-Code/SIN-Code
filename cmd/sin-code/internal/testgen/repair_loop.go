// SPDX-License-Identifier: MIT
// Purpose: Generate→compile→execute→repair loop for LLM-driven test
// generation. Wraps LLMFiller in a bounded retry cycle: each round
// generates (or repairs) a test file, compiles it, runs it, and feeds
// failures back to the LLM for the next round. MaxRounds bounds the
// cycle (default 3, safety cap 10).
//
// Issue #256: wires the LLM case filling and generate/execute/repair loop.
package testgen

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

// DefaultRepairRounds is the default upper bound on repair iterations.
const DefaultRepairRounds = 3

// maxRepairRoundsCap is the safety ceiling so a misconfigured caller
// cannot pin the loop.
const maxRepairRoundsCap = 10

// RepairRequest describes a single repair-loop run.
type RepairRequest struct {
	SourceFile   string
	FunctionName string
	MaxRounds    int
}

// RepairResult reports the final outcome of a repair loop.
type RepairResult struct {
	TestCode      string `json:"test_code"`
	RoundsUsed    int    `json:"rounds_used"`
	FinalPass     bool   `json:"final_pass"`
	CompileErrors string `json:"compile_errors,omitempty"`
	TestResults   string `json:"test_results,omitempty"`
}

// CompileFunc compiles test files in dir. Returns combined output and
// an error when compilation fails.
type CompileFunc func(ctx context.Context, dir string) (string, error)

// RunTestFunc executes tests in dir. Returns combined output and
// whether the run passed.
type RunTestFunc func(ctx context.Context, dir string) (string, bool)

// RepairLoopOption configures a RepairLoop at construction time.
type RepairLoopOption func(*RepairLoop)

// RepairLoop orchestrates the generate→compile→execute→repair cycle.
type RepairLoop struct {
	filler      *LLMFiller
	maxRounds   int
	timeout     time.Duration
	compileFunc CompileFunc
	runTestFunc RunTestFunc
	writeFile   func(path string, data []byte) error
}

// NewRepairLoop constructs a RepairLoop with sensible defaults. The
// compile and test runners are swappable via options for testability.
func NewRepairLoop(filler *LLMFiller, opts ...RepairLoopOption) *RepairLoop {
	l := &RepairLoop{
		filler:      filler,
		maxRounds:   DefaultRepairRounds,
		timeout:     DefaultTimeout,
		compileFunc: defaultCompileFunc,
		runTestFunc: defaultRunTestFunc,
		writeFile:   func(p string, d []byte) error { return os.WriteFile(p, d, filemode.Default()) },
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// WithRepairMaxRounds sets the default max rounds (overridden by
// RepairRequest.MaxRounds when non-zero).
func WithRepairMaxRounds(n int) RepairLoopOption {
	return func(l *RepairLoop) {
		if n > 0 {
			l.maxRounds = n
		}
	}
}

// WithRepairTimeout sets the wall-clock budget for the entire loop.
func WithRepairTimeout(d time.Duration) RepairLoopOption {
	return func(l *RepairLoop) {
		if d > 0 {
			l.timeout = d
		}
	}
}

// WithCompileFunc overrides the compile runner (test seam).
func WithCompileFunc(fn CompileFunc) RepairLoopOption {
	return func(l *RepairLoop) {
		if fn != nil {
			l.compileFunc = fn
		}
	}
}

// WithRunTestFunc overrides the test runner (test seam).
func WithRunTestFunc(fn RunTestFunc) RepairLoopOption {
	return func(l *RepairLoop) {
		if fn != nil {
			l.runTestFunc = fn
		}
	}
}

// WithWriteFileFunc overrides the file writer (test seam).
func WithWriteFileFunc(fn func(string, []byte) error) RepairLoopOption {
	return func(l *RepairLoop) {
		if fn != nil {
			l.writeFile = fn
		}
	}
}

// Run executes the generate→compile→execute→repair cycle up to
// MaxRounds times. It returns a RepairResult describing the final
// state regardless of pass/fail — the caller inspects FinalPass.
func (l *RepairLoop) Run(ctx context.Context, req RepairRequest) (*RepairResult, error) {
	if l == nil || l.filler == nil {
		return nil, fmt.Errorf("testgen: RepairLoop filler is nil")
	}
	if req.SourceFile == "" {
		return nil, fmt.Errorf("testgen: RepairRequest.SourceFile is required")
	}

	maxRounds := req.MaxRounds
	if maxRounds <= 0 {
		maxRounds = l.maxRounds
	}
	if maxRounds > maxRepairRoundsCap {
		maxRounds = maxRepairRoundsCap
	}

	cctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	testFile := deriveTestFileName(req.SourceFile)
	dir := filepath.Dir(req.SourceFile)
	if dir == "" {
		dir = "."
	}

	result := &RepairResult{}
	var prevCode, failingOutput string

	for round := 1; round <= maxRounds; round++ {
		if cctx.Err() != nil {
			break
		}
		result.RoundsUsed = round

		fillReq := FillRequest{
			SourceFile:    req.SourceFile,
			FunctionName:  req.FunctionName,
			ExistingTests: prevCode,
			Language:      "go",
			MaxCases:      5,
		}
		if round > 1 && failingOutput != "" {
			fillReq.ExistingTests = prevCode + "\n\n# Previous failing output:\n" + failingOutput
		}

		fillRes, err := l.filler.Fill(cctx, fillReq)
		if err != nil {
			result.CompileErrors = fmt.Sprintf("round %d fill failed: %v", round, err)
			continue
		}

		if err := l.writeFile(testFile, []byte(fillRes.TestCode)); err != nil {
			result.CompileErrors = fmt.Sprintf("round %d write failed: %v", round, err)
			continue
		}
		result.TestCode = fillRes.TestCode
		prevCode = fillRes.TestCode

		compileOut, compileErr := l.compileFunc(cctx, dir)
		if compileErr != nil {
			result.CompileErrors = compileOut
			failingOutput = compileOut
			continue
		}
		result.CompileErrors = ""

		testOut, passed := l.runTestFunc(cctx, dir)
		result.TestResults = testOut
		if passed {
			result.FinalPass = true
			return result, nil
		}
		failingOutput = testOut
	}

	return result, nil
}

// defaultCompileFunc compiles test files without running tests.
// `go test -run ^$` compiles the test binary but matches no tests,
// giving a fast compile-only check.
func defaultCompileFunc(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-run", "^$", "-count=1", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// defaultRunTestFunc runs the full test suite in dir.
func defaultRunTestFunc(ctx context.Context, dir string) (string, bool) {
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-timeout=60s", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}
