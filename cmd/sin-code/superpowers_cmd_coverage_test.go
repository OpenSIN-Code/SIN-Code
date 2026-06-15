// SPDX-License-Identifier: MIT
// Purpose: coverage tests for superpowers_cmd.go — every subcommand and
// every branch of the helper functions is exercised through package-level
// hooks so no real network, git, or user filesystem state is required.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/superpowers"
	"github.com/spf13/cobra"
)

// setSpHook replaces a package-level hook for the duration of a test and
// restores the original value on cleanup.
func setSpHook[T any](t *testing.T, ptr *T, val T) {
	orig := *ptr
	*ptr = val
	t.Cleanup(func() { *ptr = orig })
}

// runCmdWithOutput executes a cobra command with stdout/stderr captured
// so assertions can inspect both the structured RunE output and cobra's
// own error emission.
func runCmdWithOutput(t *testing.T, cmd *cobra.Command) (stdout, stderr string, err error) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	os.Stdout, os.Stderr = wOut, wErr
	var bOut, bErr strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { io.Copy(&bOut, rOut); wg.Done() }()
	go func() { io.Copy(&bErr, rErr); wg.Done() }()
	cmd.SetOut(wOut)
	cmd.SetErr(wErr)
	err = cmd.Execute()
	wOut.Close()
	wErr.Close()
	wg.Wait()
	os.Stdout, os.Stderr = origOut, origErr
	return bOut.String(), bErr.String(), err
}

func TestNewSuperpowersCmd(t *testing.T) {
	cmd := NewSuperpowersCmd()
	if cmd.Use != "superpowers" {
		t.Errorf("Use = %q, want superpowers", cmd.Use)
	}
	names := []string{}
	for _, c := range cmd.Commands() {
		names = append(names, c.Use)
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"install", "update", "pin", "list", "show", "find", "serve", "init", "doctor"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing subcommand %q in %q", want, joined)
		}
	}
}

func TestSuperpowers_Install(t *testing.T) {
	res := &superpowers.InstallResult{
		Repo:     "https://example.com/superpowers.git",
		SHA:      "abcdef1234567890abcdef1234567890",
		Branch:   "main",
		Skills:   3,
		Duration: "1s",
	}
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "/tmp/mcp.json", nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"install"})
	out, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v (stderr: %q)", err, errOut)
	}
	if !strings.Contains(out, "installed 3 skill(s)") {
		t.Errorf("missing install summary: %q", out)
	}
	if !strings.Contains(out, "abcdef12") {
		t.Errorf("missing SHA: %q", out)
	}
	if strings.Contains(errOut, "warning") {
		t.Errorf("unexpected warning: %q", errOut)
	}
}

func TestSuperpowers_InstallJSON(t *testing.T) {
	res := &superpowers.InstallResult{
		Repo:     "https://example.com/superpowers.git",
		SHA:      "abcdef1234567890abcdef1234567890",
		Branch:   "main",
		Skills:   5,
		Duration: "2s",
	}
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "/tmp/mcp.json", nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"install", "--json"})
	out, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v (stderr: %q)", err, errOut)
	}
	var got superpowers.InstallResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json output invalid: %v: %q", err, out)
	}
	if got.Skills != 5 || got.Branch != "main" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestSuperpowers_InstallError(t *testing.T) {
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return nil, errors.New("install failed")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"install"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "install failed") {
		t.Fatalf("expected install error, got %v", err)
	}
}

func TestSuperpowers_InstallMCPWarning(t *testing.T) {
	res := &superpowers.InstallResult{Skills: 1, Duration: "1s"}
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "", errors.New("mcp boom")
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"install"})
	_, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(errOut, "MCP register failed") {
		t.Errorf("missing MCP warning: %q", errOut)
	}
}

func TestSuperpowers_InstallAgentsInjection(t *testing.T) {
	res := &superpowers.InstallResult{Skills: 2, Duration: "1s"}
	injected := false
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "/tmp/mcp.json", nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{{Name: "x"}}, nil
	})
	setSpHook(t, &superpowersAGENTSSnippetHook, func(skills []superpowers.SkillInfo) string {
		return "agents snippet"
	})
	setSpHook(t, &superpowersInjectAGENTSHook, func(path, snippet string) error {
		injected = true
		return nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"install", "--agents", "/tmp/AGENTS.md"})
	_, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !injected {
		t.Error("agents injection not called")
	}
	if !strings.Contains(errOut, "injected superpowers block") {
		t.Errorf("missing injection message: %q", errOut)
	}
}

func TestSuperpowers_InstallAgentsInjectionWarning(t *testing.T) {
	res := &superpowers.InstallResult{Skills: 2, Duration: "1s"}
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "/tmp/mcp.json", nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})
	setSpHook(t, &superpowersAGENTSSnippetHook, func(skills []superpowers.SkillInfo) string {
		return "agents snippet"
	})
	setSpHook(t, &superpowersInjectAGENTSHook, func(path, snippet string) error {
		return errors.New("inject failed")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"install", "--agents", "/tmp/AGENTS.md"})
	_, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(errOut, "AGENTS.md injection failed") {
		t.Errorf("missing injection warning: %q", errOut)
	}
}

func TestSuperpowers_UpdateNoYes(t *testing.T) {
	res := &superpowers.InstallResult{Skills: 1, Duration: "1s"}
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "/tmp/mcp.json", nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"update"})
	out, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(errOut, "running `superpowers update`") {
		t.Errorf("missing update prompt: %q", errOut)
	}
	if !strings.Contains(out, "updated 1 skill(s)") {
		t.Errorf("missing update summary: %q", out)
	}
}

func TestSuperpowers_UpdateYes(t *testing.T) {
	res := &superpowers.InstallResult{Skills: 1, Duration: "1s"}
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "/tmp/mcp.json", nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"update", "--yes"})
	out, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if strings.Contains(errOut, "running `superpowers update`") {
		t.Errorf("unexpected update prompt: %q", errOut)
	}
	if !strings.Contains(out, "updated 1 skill(s)") {
		t.Errorf("missing update summary: %q", out)
	}
}

func TestSuperpowers_UpdateError(t *testing.T) {
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return nil, errors.New("update install failed")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"update", "--yes"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "update install failed") {
		t.Fatalf("expected update error, got %v", err)
	}
}

func TestSuperpowers_UpdateMCPWarning(t *testing.T) {
	res := &superpowers.InstallResult{Skills: 1, Duration: "1s"}
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "", errors.New("mcp boom")
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"update", "--yes"})
	_, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(errOut, "MCP register failed") {
		t.Errorf("missing MCP warning: %q", errOut)
	}
}

func TestSuperpowers_UpdateAgentsInjection(t *testing.T) {
	res := &superpowers.InstallResult{Skills: 2, Duration: "1s"}
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "/tmp/mcp.json", nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{{Name: "x"}}, nil
	})
	setSpHook(t, &superpowersAGENTSSnippetHook, func(skills []superpowers.SkillInfo) string {
		return "agents snippet"
	})
	setSpHook(t, &superpowersInjectAGENTSHook, func(path, snippet string) error {
		return errors.New("inject failed")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"update", "--yes", "--agents", "/tmp/AGENTS.md"})
	_, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(errOut, "AGENTS.md injection failed") {
		t.Errorf("missing injection warning: %q", errOut)
	}
}

func TestSuperpowers_UpdateJSON(t *testing.T) {
	res := &superpowers.InstallResult{Skills: 4, Duration: "1s"}
	setSpHook(t, &superpowersInstallHook, func(ctx context.Context, repo, branch string) (*superpowers.InstallResult, error) {
		return res, nil
	})
	setSpHook(t, &superpowersRegisterMCPHook, func(mcpPath string) (string, error) {
		return "/tmp/mcp.json", nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"update", "--yes", "--json"})
	out, errOut, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v (stderr: %q)", err, errOut)
	}
	var got superpowers.InstallResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json output invalid: %v: %q", err, out)
	}
	if got.Skills != 4 {
		t.Errorf("unexpected skills: %d", got.Skills)
	}
}

func TestSuperpowers_Pin(t *testing.T) {
	setSpHook(t, &superpowersPinHook, func(ctx context.Context, sha string) (*superpowers.PinState, error) {
		return &superpowers.PinState{
			SHA:       "abcdef1234567890",
			Branch:    "main",
			UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		}, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"pin", "abcdef1234567890"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "pinned to abcdef1234567890") {
		t.Errorf("missing pin output: %q", out)
	}
	if !strings.Contains(out, "2026-01-02T03:04:05Z") {
		t.Errorf("missing timestamp: %q", out)
	}
}

func TestSuperpowers_PinError(t *testing.T) {
	setSpHook(t, &superpowersPinHook, func(ctx context.Context, sha string) (*superpowers.PinState, error) {
		return nil, errors.New("pin failed")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"pin", "abc"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "pin failed") {
		t.Fatalf("expected pin error, got %v", err)
	}
}

func TestSuperpowers_ListEmpty(t *testing.T) {
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"list"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "no skills installed") {
		t.Errorf("missing empty message: %q", out)
	}
}

func TestSuperpowers_ListEmptyJSON(t *testing.T) {
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{}, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"list", "--json"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "[]") {
		t.Errorf("expected empty JSON array: %q", out)
	}
}

func TestSuperpowers_ListNonEmpty(t *testing.T) {
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{
			{Name: "alpha", Hash: "1234567890abcdef", Path: "/a/alpha/SKILL.md"},
			{Name: "beta-long-name", Hash: "fedcba0987654321", Path: "/a/beta/SKILL.md"},
		}, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"list"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "alpha") {
		t.Errorf("missing alpha: %q", out)
	}
	if !strings.Contains(out, "12345678") {
		t.Errorf("missing truncated hash: %q", out)
	}
}

func TestSuperpowers_ListError(t *testing.T) {
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, errors.New("list failed")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"list"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "list failed") {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestSuperpowers_Show(t *testing.T) {
	setSpHook(t, &superpowersGetHook, func(name string) (*superpowers.SkillInfo, error) {
		return &superpowers.SkillInfo{Name: "alpha", Path: "/a/alpha/SKILL.md"}, nil
	})
	setSpHook(t, &osReadFileHook, func(path string) ([]byte, error) {
		return []byte("# alpha\nbody\n"), nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"show", "alpha"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "body") {
		t.Errorf("missing body: %q", out)
	}
}

func TestSuperpowers_ShowGetError(t *testing.T) {
	setSpHook(t, &superpowersGetHook, func(name string) (*superpowers.SkillInfo, error) {
		return nil, errors.New("not found")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"show", "alpha"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected show error, got %v", err)
	}
}

func TestSuperpowers_ShowReadError(t *testing.T) {
	setSpHook(t, &superpowersGetHook, func(name string) (*superpowers.SkillInfo, error) {
		return &superpowers.SkillInfo{Name: "alpha", Path: "/a/alpha/SKILL.md"}, nil
	})
	setSpHook(t, &osReadFileHook, func(path string) ([]byte, error) {
		return nil, errors.New("read denied")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"show", "alpha"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "read denied") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestSuperpowers_FindEmpty(t *testing.T) {
	setSpHook(t, &superpowersFindHook, func(query string, maxResults int) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"find", "foo"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `no skills match "foo"`) {
		t.Errorf("missing empty message: %q", out)
	}
}

func TestSuperpowers_FindNonEmpty(t *testing.T) {
	setSpHook(t, &superpowersFindHook, func(query string, maxResults int) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{
			{Name: "alpha", Description: "does alpha things"},
			{Name: "beta", Description: ""},
		}, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"find", "foo"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "alpha: does alpha things") {
		t.Errorf("missing alpha: %q", out)
	}
	if !strings.Contains(out, "beta: (no description)") {
		t.Errorf("missing beta description fallback: %q", out)
	}
}

func TestSuperpowers_FindJSON(t *testing.T) {
	setSpHook(t, &superpowersFindHook, func(query string, maxResults int) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{{Name: "alpha"}}, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"find", "foo", "--json"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []superpowers.SkillInfo
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json output invalid: %v: %q", err, out)
	}
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Errorf("unexpected result: %+v", got)
	}
}

func TestSuperpowers_FindError(t *testing.T) {
	setSpHook(t, &superpowersFindHook, func(query string, maxResults int) ([]superpowers.SkillInfo, error) {
		return nil, errors.New("find failed")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"find", "foo"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "find failed") {
		t.Fatalf("expected find error, got %v", err)
	}
}

func TestSuperpowers_Serve(t *testing.T) {
	setSpHook(t, &superpowersNewServerHook, func(cfgDir string) *superpowers.Server {
		return superpowers.NewServerWithIO(strings.NewReader(""), io.Discard, io.Discard, "")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"serve"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestSuperpowers_ServeError(t *testing.T) {
	setSpHook(t, &superpowersNewServerHook, func(cfgDir string) *superpowers.Server {
		req := `{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"
		return superpowers.NewServerWithIO(strings.NewReader(req), &errorWriter{errors.New("write fail")}, io.Discard, "")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"serve"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "write fail") {
		t.Fatalf("expected serve error, got %v", err)
	}
}

type errorWriter struct {
	err error
}

func (e *errorWriter) Write(p []byte) (int, error) {
	return 0, e.err
}

func TestSuperpowers_InitNoArgs(t *testing.T) {
	dir := t.TempDir()
	setSpHook(t, &filepathAbsHook, func(path string) (string, error) {
		return dir, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"init"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "scaffolded") {
		t.Errorf("missing scaffolded message: %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not created: %v", err)
	}
}

func TestSuperpowers_InitWithPath(t *testing.T) {
	dir := t.TempDir()
	setSpHook(t, &filepathAbsHook, func(path string) (string, error) {
		return dir, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"init", "some/path"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, dir) {
		t.Errorf("missing path in output: %q", out)
	}
}

func TestSuperpowers_InitAbsError(t *testing.T) {
	setSpHook(t, &filepathAbsHook, func(path string) (string, error) {
		return "", errors.New("abs fail")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"init", "/tmp"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "abs fail") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestSuperpowers_InitMkdirError(t *testing.T) {
	setSpHook(t, &filepathAbsHook, func(path string) (string, error) {
		return "/tmp/init-test", nil
	})
	setSpHook(t, &osMkdirAllHook, func(path string, perm os.FileMode) error {
		return errors.New("mkdir fail")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"init", "/tmp"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "mkdir fail") {
		t.Fatalf("expected mkdir error, got %v", err)
	}
}

func TestSuperpowers_InitExistsError(t *testing.T) {
	setSpHook(t, &filepathAbsHook, func(path string) (string, error) {
		return "/tmp/init-exists", nil
	})
	setSpHook(t, &osMkdirAllHook, func(path string, perm os.FileMode) error {
		return nil
	})
	setSpHook(t, &osStatHook, func(path string) (os.FileInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"init", "/tmp"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected exists error, got %v", err)
	}
}

func TestSuperpowers_InitWriteError(t *testing.T) {
	setSpHook(t, &filepathAbsHook, func(path string) (string, error) {
		return "/tmp/init-write", nil
	})
	setSpHook(t, &osMkdirAllHook, func(path string, perm os.FileMode) error {
		return nil
	})
	setSpHook(t, &osStatHook, func(path string) (os.FileInfo, error) {
		return nil, errors.New("not found")
	})
	setSpHook(t, &osWriteFileHook, func(path string, data []byte, perm os.FileMode) error {
		return errors.New("write fail")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"init", "/tmp"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "write fail") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestSuperpowers_DoctorAllOK(t *testing.T) {
	setSpHook(t, &superpowersSkillsDirHook, func() string { return "/tmp/sp/skills" })
	setSpHook(t, &superpowersCurrentPinHook, func() (*superpowers.PinState, error) {
		return &superpowers.PinState{SHA: "abcdef1234567890"}, nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{{Name: "alpha", Path: "/a/SKILL.md"}}, nil
	})
	setSpHook(t, &osReadFileHook, func(path string) ([]byte, error) {
		return []byte("body\n" + superpowers.OverlayMarker + "\n"), nil
	})
	setSpHook(t, &superpowersOverlayMarkerHook, superpowers.OverlayMarker)
	setSpHook(t, &superpowersMCPConfigPathHook, func() string { return "/tmp/sp/mcp.json" })
	setSpHook(t, &superpowersPROMPTFileHook, func() string { return "/tmp/sp/PROMPT.md" })
	setSpHook(t, &osStatHook, func(path string) (os.FileInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"doctor"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "OK") {
		t.Errorf("missing OK marker: %q", out)
	}
	if strings.Contains(out, "FAIL") {
		t.Errorf("unexpected FAIL: %q", out)
	}
}

func TestSuperpowers_DoctorAllOKJSON(t *testing.T) {
	setSpHook(t, &superpowersSkillsDirHook, func() string { return "/tmp/sp/skills" })
	setSpHook(t, &superpowersCurrentPinHook, func() (*superpowers.PinState, error) {
		return &superpowers.PinState{SHA: "abcdef1234567890"}, nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{{Name: "alpha", Path: "/a/SKILL.md"}}, nil
	})
	setSpHook(t, &osReadFileHook, func(path string) ([]byte, error) {
		return []byte("body\n" + superpowers.OverlayMarker + "\n"), nil
	})
	setSpHook(t, &superpowersOverlayMarkerHook, superpowers.OverlayMarker)
	setSpHook(t, &superpowersMCPConfigPathHook, func() string { return "/tmp/sp/mcp.json" })
	setSpHook(t, &superpowersPROMPTFileHook, func() string { return "/tmp/sp/PROMPT.md" })
	setSpHook(t, &osStatHook, func(path string) (os.FileInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"doctor", "--json"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json output invalid: %v: %q", err, out)
	}
	if got["all_ok"] != true {
		t.Errorf("expected all_ok=true, got %v", got["all_ok"])
	}
}

func TestSuperpowers_DoctorFailures(t *testing.T) {
	setSpHook(t, &superpowersSkillsDirHook, func() string { return "/tmp/sp/skills" })
	setSpHook(t, &superpowersCurrentPinHook, func() (*superpowers.PinState, error) {
		return nil, errors.New("pin read error")
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})
	setSpHook(t, &superpowersMCPConfigPathHook, func() string { return "/tmp/sp/mcp.json" })
	setSpHook(t, &superpowersPROMPTFileHook, func() string { return "/tmp/sp/PROMPT.md" })
	setSpHook(t, &osStatHook, func(path string) (os.FileInfo, error) {
		return nil, errors.New("missing")
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"doctor"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "one or more checks failed") {
		t.Fatalf("expected doctor failure, got %v", err)
	}
}

func TestSuperpowers_DoctorOverlayMissing(t *testing.T) {
	setSpHook(t, &superpowersSkillsDirHook, func() string { return "/tmp/sp/skills" })
	setSpHook(t, &superpowersCurrentPinHook, func() (*superpowers.PinState, error) {
		return &superpowers.PinState{SHA: "abc"}, nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{{Name: "alpha", Path: "/a/SKILL.md"}}, nil
	})
	setSpHook(t, &osReadFileHook, func(path string) ([]byte, error) {
		return []byte("body without overlay\n"), nil
	})
	setSpHook(t, &superpowersOverlayMarkerHook, superpowers.OverlayMarker)
	setSpHook(t, &superpowersMCPConfigPathHook, func() string { return "/tmp/sp/mcp.json" })
	setSpHook(t, &superpowersPROMPTFileHook, func() string { return "/tmp/sp/PROMPT.md" })
	setSpHook(t, &osStatHook, func(path string) (os.FileInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"doctor"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "one or more checks failed") {
		t.Fatalf("expected doctor failure, got %v", err)
	}
	if !strings.Contains(out, "1/1 missing overlay") {
		t.Errorf("missing overlay detail: %q", out)
	}
}

func TestSuperpowers_DoctorPinNil(t *testing.T) {
	setSpHook(t, &superpowersSkillsDirHook, func() string { return "/tmp/sp/skills" })
	setSpHook(t, &superpowersCurrentPinHook, func() (*superpowers.PinState, error) {
		return nil, nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return nil, nil
	})
	setSpHook(t, &superpowersMCPConfigPathHook, func() string { return "/tmp/sp/mcp.json" })
	setSpHook(t, &superpowersPROMPTFileHook, func() string { return "/tmp/sp/PROMPT.md" })
	setSpHook(t, &osStatHook, func(path string) (os.FileInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"doctor"})
	_, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "one or more checks failed") {
		t.Fatalf("expected doctor failure, got %v", err)
	}
}

func TestSuperpowers_DoctorReadError(t *testing.T) {
	setSpHook(t, &superpowersSkillsDirHook, func() string { return "/tmp/sp/skills" })
	setSpHook(t, &superpowersCurrentPinHook, func() (*superpowers.PinState, error) {
		return &superpowers.PinState{SHA: "abc"}, nil
	})
	setSpHook(t, &superpowersListHook, func(root string) ([]superpowers.SkillInfo, error) {
		return []superpowers.SkillInfo{{Name: "alpha", Path: "/a/SKILL.md"}}, nil
	})
	setSpHook(t, &osReadFileHook, func(path string) ([]byte, error) {
		return nil, errors.New("read error")
	})
	setSpHook(t, &superpowersOverlayMarkerHook, superpowers.OverlayMarker)
	setSpHook(t, &superpowersMCPConfigPathHook, func() string { return "/tmp/sp/mcp.json" })
	setSpHook(t, &superpowersPROMPTFileHook, func() string { return "/tmp/sp/PROMPT.md" })
	setSpHook(t, &osStatHook, func(path string) (os.FileInfo, error) {
		return nil, nil
	})

	cmd := NewSuperpowersCmd()
	cmd.SetArgs([]string{"doctor"})
	out, _, err := runCmdWithOutput(t, cmd)
	if err == nil || !strings.Contains(err.Error(), "one or more checks failed") {
		t.Fatalf("expected doctor failure, got %v", err)
	}
	if !strings.Contains(out, "1/1 missing overlay") {
		t.Errorf("missing overlay detail: %q", out)
	}
}

func TestSuperpowers_Min(t *testing.T) {
	if min(1, 2) != 1 {
		t.Errorf("min(1,2) = %d", min(1, 2))
	}
	if min(2, 1) != 1 {
		t.Errorf("min(2,1) = %d", min(2, 1))
	}
}
