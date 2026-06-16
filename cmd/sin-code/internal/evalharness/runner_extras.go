// SPDX-License-Identifier: MIT
// Purpose: helpers for CompileAndRun scorer — code-block extraction,
// per-language compile, and sandboxed self-check execution.
// Docs: runner_extras.doc.md
package evalharness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sandbox"
)

// codeBlockRe matches the first fenced ```...``` block. It tolerates an
// info string (e.g. ```python) and captures the inner content.
var codeBlockRe = regexp.MustCompile("(?s)```(?:\\w+)?\\n?(.*?)\\n?```")

// extractCodeBlock returns the first fenced code block in s, or "" if none.
func extractCodeBlock(s string) string {
	m := codeBlockRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// compile validates the code by spawning the language's syntax checker.
// It uses the sandbox package so untrusted code runs with limited access.
func (c CompileAndRun) compile(code string, timeout time.Duration) error {
	tmpDir, err := os.MkdirTemp("", "sin-eval-compile-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	policy := sandbox.DefaultPolicy(tmpDir, tmpDir)
	policy.Timeout = timeout

	switch c.Language {
	case "python":
		return c.compilePython(ctx, policy, tmpDir, code)
	case "go":
		return c.compileGo(ctx, policy, tmpDir, code)
	case "javascript":
		return c.compileJavaScript(ctx, policy, tmpDir, code)
	case "bash":
		return c.compileBash(ctx, policy, tmpDir, code)
	default:
		return fmt.Errorf("unsupported language: %q", c.Language)
	}
}

// run executes the code with selfCheck appended, using the sandbox.
// For Go the self-check is written into a separate file in the same package.
func (c CompileAndRun) run(code, selfCheck string, timeout time.Duration) error {
	tmpDir, err := os.MkdirTemp("", "sin-eval-run-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	policy := sandbox.DefaultPolicy(tmpDir, tmpDir)
	policy.Timeout = timeout

	switch c.Language {
	case "python":
		return c.runPython(ctx, policy, tmpDir, code, selfCheck)
	case "go":
		return c.runGo(ctx, policy, tmpDir, code, selfCheck)
	case "javascript":
		return c.runJavaScript(ctx, policy, tmpDir, code, selfCheck)
	case "bash":
		return c.runBash(ctx, policy, tmpDir, code, selfCheck)
	default:
		return fmt.Errorf("unsupported language: %q", c.Language)
	}
}

// compilePython runs python3 -m py_compile on the source file.
func (c CompileAndRun) compilePython(ctx context.Context, policy sandbox.Policy, tmpDir, code string) error {
	src := filepath.Join(tmpDir, "solution.py")
	if err := os.WriteFile(src, []byte(code), 0600); err != nil {
		return err
	}
	cmd, res, err := sandbox.Command(ctx, policy, c.interpreter("python3", "python"), "-m", "py_compile", src)
	if err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, sandboxOutput(res, out))
	}
	return nil
}

// runPython appends the self-check and runs the combined file.
func (c CompileAndRun) runPython(ctx context.Context, policy sandbox.Policy, tmpDir, code, selfCheck string) error {
	src := filepath.Join(tmpDir, "solution.py")
	full := code + "\n" + selfCheck + "\n"
	if err := os.WriteFile(src, []byte(full), 0600); err != nil {
		return err
	}
	cmd, res, err := sandbox.Command(ctx, policy, c.interpreter("python3", "python"), src)
	if err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, sandboxOutput(res, out))
	}
	return nil
}

// compileGo creates a tiny module, writes the code, and builds it.
func (c CompileAndRun) compileGo(ctx context.Context, policy sandbox.Policy, tmpDir, code string) error {
	if err := initGoModule(tmpDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "solution.go"), []byte(code), 0600); err != nil {
		return err
	}
	cmd, res, err := sandbox.Command(ctx, policy, c.interpreter("go", "go"), "build", "-o", "solution", ".")
	if err != nil {
		return err
	}
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, sandboxOutput(res, out))
	}
	return nil
}

// runGo appends the self-check as a separate main file and runs the module.
func (c CompileAndRun) runGo(ctx context.Context, policy sandbox.Policy, tmpDir, code, selfCheck string) error {
	if err := initGoModule(tmpDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "solution.go"), []byte(code), 0600); err != nil {
		return err
	}
	mainFile := filepath.Join(tmpDir, "main.go")
	mainBody := selfCheck
	if !strings.HasPrefix(strings.TrimSpace(selfCheck), "package ") {
		mainBody = "package main\n" + mainBody
	}
	if err := os.WriteFile(mainFile, []byte(mainBody), 0600); err != nil {
		return err
	}
	cmd, res, err := sandbox.Command(ctx, policy, c.interpreter("go", "go"), "run", ".")
	if err != nil {
		return err
	}
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, sandboxOutput(res, out))
	}
	return nil
}

// compileJavaScript runs node --check on the source file.
func (c CompileAndRun) compileJavaScript(ctx context.Context, policy sandbox.Policy, tmpDir, code string) error {
	src := filepath.Join(tmpDir, "solution.js")
	if err := os.WriteFile(src, []byte(code), 0600); err != nil {
		return err
	}
	cmd, res, err := sandbox.Command(ctx, policy, c.interpreter("node", "node"), "--check", src)
	if err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, sandboxOutput(res, out))
	}
	return nil
}

// runJavaScript appends the self-check and runs the combined file.
func (c CompileAndRun) runJavaScript(ctx context.Context, policy sandbox.Policy, tmpDir, code, selfCheck string) error {
	src := filepath.Join(tmpDir, "solution.js")
	full := code + "\n" + selfCheck + "\n"
	if err := os.WriteFile(src, []byte(full), 0600); err != nil {
		return err
	}
	cmd, res, err := sandbox.Command(ctx, policy, c.interpreter("node", "node"), src)
	if err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, sandboxOutput(res, out))
	}
	return nil
}

// compileBash runs bash -n for syntax checking.
func (c CompileAndRun) compileBash(ctx context.Context, policy sandbox.Policy, tmpDir, code string) error {
	src := filepath.Join(tmpDir, "solution.sh")
	if err := os.WriteFile(src, []byte(code), 0600); err != nil {
		return err
	}
	cmd, res, err := sandbox.Command(ctx, policy, c.interpreter("bash", "bash"), "-n", src)
	if err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, sandboxOutput(res, out))
	}
	return nil
}

// runBash appends the self-check and runs the combined script.
func (c CompileAndRun) runBash(ctx context.Context, policy sandbox.Policy, tmpDir, code, selfCheck string) error {
	src := filepath.Join(tmpDir, "solution.sh")
	full := code + "\n" + selfCheck + "\n"
	if err := os.WriteFile(src, []byte(full), 0600); err != nil {
		return err
	}
	cmd, res, err := sandbox.Command(ctx, policy, c.interpreter("bash", "bash"), src)
	if err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, sandboxOutput(res, out))
	}
	return nil
}

// interpreter returns the explicit Binary if set, otherwise tries the
// preferred names and falls back to the bare command name.
func (c CompileAndRun) interpreter(preferred, fallback string) string {
	if c.Binary != "" {
		return c.Binary
	}
	if _, err := exec.LookPath(preferred); err == nil {
		return preferred
	}
	return fallback
}

// initGoModule creates a minimal go.mod in dir so `go build` / `go run`
// work without an enclosing module.
func initGoModule(dir string) error {
	mod := filepath.Join(dir, "go.mod")
	return os.WriteFile(mod, []byte("module sin\n\ngo 1.23\n"), 0600)
}

// sandboxOutput decorates command output with the sandbox mechanism.
func sandboxOutput(res sandbox.Result, out []byte) string {
	s := strings.TrimSpace(string(out))
	if s == "" {
		s = "<no output>"
	}
	if res.Warning != "" {
		return fmt.Sprintf("%s (sandbox: %s)", s, res.Warning)
	}
	return fmt.Sprintf("%s (sandbox: %s)", s, res.Mechanism)
}
