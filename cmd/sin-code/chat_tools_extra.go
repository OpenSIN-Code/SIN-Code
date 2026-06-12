// SPDX-License-Identifier: MIT
// Purpose: extended builtin tools — git operations (read-only allow,
// mutating ask), bounded HTTP fetch, and test runner. Closes the gap
// between "needs sin_bash" and "needs a real tool".
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxHTTPBytes = 256 * 1024
	gitTimeout   = 30 * time.Second
	testTimeout  = 5 * time.Minute
)

// extraSpecs is appended to builtinSpecs() in chat_tools.go.
func extraSpecs() []agentloopToolSpecAlias {
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }
	obj := func(p map[string]any, req ...string) map[string]any {
		return map[string]any{"type": "object", "properties": p, "required": req}
	}
	return []agentloopToolSpecAlias{
		{Name: "sin_git_log", Description: "Show recent commit history (read-only).",
			InputSchema: obj(map[string]any{"limit": str("number of commits, default 10"), "path": str("optional path filter")})},
		{Name: "sin_git_diff", Description: "Show working tree diff or diff vs a ref (read-only).",
			InputSchema: obj(map[string]any{"ref": str("optional ref to diff against, default working tree")})},
		{Name: "sin_git_commit", Description: "Stage all changes and commit with a message (mutating — gated).",
			InputSchema: obj(map[string]any{"message": str("conventional commit message")}, "message")},
		{Name: "sin_http_get", Description: "Fetch a URL (GET only, 256KB cap, 30s timeout). For docs/APIs.",
			InputSchema: obj(map[string]any{"url": str("http(s) URL")}, "url")},
		{Name: "sin_test", Description: "Run the workspace test suite and return structured pass/fail output.",
			InputSchema: obj(map[string]any{"target": str("optional package/file filter")})},
	}
}

// extraTool is called from builtinTool()'s default branch.
func extraTool(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case "sin_git_log":
		n := argStr(args, "limit")
		if n == "" {
			n = "10"
		}
		a := []string{"log", "--oneline", "--decorate", "-n", n}
		if p := argStr(args, "path"); p != "" {
			a = append(a, "--", p)
		}
		return runGit(ctx, a...)
	case "sin_git_diff":
		if ref := argStr(args, "ref"); ref != "" {
			return runGit(ctx, "diff", ref, "--stat", "-p")
		}
		return runGit(ctx, "diff", "--stat", "-p")
	case "sin_git_commit":
		msg := argStr(args, "message")
		if msg == "" {
			return "", fmt.Errorf("sin_git_commit: message required")
		}
		if out, err := runGit(ctx, "add", "-A"); err != nil {
			return out, err
		}
		return runGit(ctx, "commit", "-m", msg)
	case "sin_http_get":
		return toolHTTPGet(ctx, argStr(args, "url"))
	case "sin_test":
		return toolTest(ctx, argStr(args, "target"))
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func runGit(ctx context.Context, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "git", args...)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	if err != nil {
		return fmt.Sprintf("git error: %v\n%s", err, text), nil
	}
	return text, nil
}

func toolHTTPGet(ctx context.Context, url string) (string, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("sin_http_get: only http(s) URLs allowed")
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sin-code-agent/3.5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPBytes))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("HTTP %d (%d bytes)\n%s", resp.StatusCode, len(body), body), nil
}

func toolTest(ctx context.Context, target string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, testTimeout)
	defer cancel()
	var cmd *exec.Cmd
	switch {
	case fileExists("go.mod"):
		pkg := "./..."
		if target != "" {
			pkg = target
		}
		cmd = exec.CommandContext(cctx, "go", "test", pkg, "-count=1")
	case fileExists("package.json"):
		cmd = exec.CommandContext(cctx, "sh", "-c", "npm test --silent 2>&1")
	case fileExists("pyproject.toml") || fileExists("pytest.ini"):
		args := []string{"-m", "pytest", "-q"}
		if target != "" {
			args = append(args, target)
		}
		cmd = exec.CommandContext(cctx, "python3", args...)
	default:
		return "", fmt.Errorf("sin_test: no recognized test setup (go.mod/package.json/pyproject.toml)")
	}
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	status := "PASS"
	if err != nil {
		status = "FAIL"
	}
	return fmt.Sprintf("TEST %s\n%s", status, text), nil
}

func fileExists(p string) bool {
	_, err := os.Stat(filepath.Join(".", p))
	return err == nil
}

// silence linter for unused json
var _ = json.Marshal
