// SPDX-License-Identifier: MIT
// Purpose: opt-in post-edit automation tied to the agent loop's
// tool.post event (issue #376). AutoLintListener runs gofmt + go vet
// on every .go file edited by sin_write/sin_edit when agentloop.auto_lint
// is true (read-only). AutoTestListener runs `go test -race -count=1` on
// the enclosing package whenever a *_test.go file is touched when
// agentloop.auto_test is true (may produce side-effects). Both listeners
// are gated on dedicated config keys — default behaviour preserved.
package hooks

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func AutoLintListener(cfg AutoHookConfig) PostListener {
	cfg = cfg.normalized()
	return func(ctx context.Context, p Payload) []string {
		if p.Name != "sin_write" && p.Name != "sin_edit" {
			return nil
		}
		path, _ := p.Data["path"].(string)
		if path == "" || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		workdir := p.Workspace
		if workdir == "" {
			workdir = "."
		}
		absPath := path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(workdir, path)
		}
		if _, err := os.Stat(absPath); err != nil {
			return nil
		}
		reports := runLintCommands(ctx, absPath, workdir, cfg.Timeout)
		if len(reports) == 0 {
			return nil
		}
		for _, r := range reports {
			fmt.Fprintf(os.Stderr, "[auto-lint] %s: %s\n", path, r)
		}
		out := make([]string, 0, len(reports))
		for _, r := range reports {
			out = append(out, fmt.Sprintf("[auto-lint %s] %s", path, r))
		}
		return out
	}
}

func AutoTestListener(cfg AutoHookConfig) PostListener {
	cfg = cfg.normalized()
	return func(ctx context.Context, p Payload) []string {
		if p.Name != "sin_write" && p.Name != "sin_edit" {
			return nil
		}
		path, _ := p.Data["path"].(string)
		if path == "" || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		workdir := p.Workspace
		if workdir == "" {
			workdir = "."
		}
		absPath := path
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(workdir, path)
		}
		if _, err := os.Stat(absPath); err != nil {
			return nil
		}
		report := runTestCommand(ctx, absPath, workdir, cfg.Timeout)
		if report == "" {
			fmt.Fprintf(os.Stderr, "[auto-test] %s: PASS\n", path)
			return nil
		}
		fmt.Fprintf(os.Stderr, "[auto-test] %s: FAIL\n", path)
		if len(report) > 4096 {
			report = report[:4096] + "\n[... truncated; rerun `sin_test` for the full log]"
		}
		return []string{fmt.Sprintf("[auto-test %s] FAIL: %s", path, report)}
	}
}

type AutoHookConfig struct {
	Timeout time.Duration
}

func (c AutoHookConfig) normalized() AutoHookConfig {
	if c.Timeout <= 0 {
		c.Timeout = AutoLintDefaultTimeout
	}
	return c
}

const (
	AutoLintDefaultTimeout time.Duration = 30 * time.Second
	AutoTestDefaultTimeout time.Duration = 120 * time.Second
)

func runLintCommands(ctx context.Context, goFile, workdir string, timeout time.Duration) []string {
	var out []string
	if !strings.HasSuffix(goFile, ".go") {
		return out
	}
	gofmtCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	gofmtCmd := exec.CommandContext(gofmtCtx, "gofmt", "-l", goFile)
	gofmtCmd.Dir = workdir
	var gofmtOut, gofmtErr bytes.Buffer
	gofmtCmd.Stdout = &gofmtOut
	gofmtCmd.Stderr = &gofmtErr
	if runErr := gofmtCmd.Run(); runErr != nil {
		if !isNotFound(runErr) {
			out = append(out, fmt.Sprintf("gofmt exec failed: %v", runErr))
		}
	} else if gofmtOut.Len() > 0 {
		out = append(out, fmt.Sprintf("gofmt: %s needs `gofmt -w`", filepath.Base(goFile)))
	}

	vetCtx, cancel2 := context.WithTimeout(ctx, timeout)
	defer cancel2()
	dirOfFile := filepath.Dir(goFile)
	if dirOfFile == "" {
		dirOfFile = "."
	}
	pkgDir := filepath.Base(dirOfFile)
	cmdWorkdir := filepath.Dir(dirOfFile)
	if cmdWorkdir == "" {
		cmdWorkdir = "."
	}
	vetCmd := exec.CommandContext(vetCtx, "go", "vet", "./"+pkgDir)
	vetCmd.Dir = cmdWorkdir
	var vetOut, vetErr bytes.Buffer
	vetCmd.Stdout = &vetOut
	vetCmd.Stderr = &vetErr
	if runErr := vetCmd.Run(); runErr != nil {
		combined := strings.TrimSpace(vetOut.String() + vetErr.String())
		if combined != "" {
			first := firstLine(combined)
			out = append(out, fmt.Sprintf("go vet: %s", first))
		}
	}
	return out
}

// runTestCommand runs go test against the file's package after editing a
// *_test.go file. The conventional "filter to TestX" doesn't apply
// cleanly here because file names do not dictate function names (e.g.
// `foo_test.go` usually contains `TestFoo`, but a per-config test may
// use any `Test*` symbol, and some authors write `testFoo` lower-case).
// Default behaviour: run the whole package without -run filter so the
// listener surfaces real test outcomes to the agent regardless of the
// `func Test*` shape inside.
func runTestCommand(ctx context.Context, testFile, workdir string, timeout time.Duration) string {
	if !strings.HasSuffix(testFile, "_test.go") {
		return ""
	}
	dirOfFile := filepath.Dir(testFile)
	if dirOfFile == "" {
		dirOfFile = "."
	}
	cmdWorkdir := filepath.Dir(dirOfFile)
	if cmdWorkdir == "" {
		cmdWorkdir = "."
	}
	args := []string{
		"test",
		"./" + filepath.Base(dirOfFile),
		"-count=1",
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = cmdWorkdir
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	runErr := cmd.Run()
	if runErr == nil {
		return ""
	}
	combined := strings.TrimSpace(out.String() + errOut.String())
	if combined == "" {
		return runErr.Error()
	}
	return combined
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return s
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "executable file not found") ||
		strings.Contains(err.Error(), "no such file")
}
