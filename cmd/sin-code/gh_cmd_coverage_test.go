// SPDX-License-Identifier: MIT
// Purpose: coverage tests for gh_cmd.go — exercises setup/doctor/run/surface/serve
// using package-level hooks so the tests never require the real `gh` binary.
// Docs: gh.doc.md
package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
)

type ghErrWriter struct{ err error }

func (e ghErrWriter) Write(p []byte) (int, error) { return 0, e.err }

func saveGhHooks(t *testing.T) {
	t.Helper()
	origRegisterMCP := ghbridgeRegisterMCPHook
	origMCPConfigPath := ghbridgeMCPConfigPathHook
	origNew := ghbridgeNewHook
	origClassify := ghbridgeClassifyHook
	origAllowedSurface := ghbridgeAllowedSurfaceHook
	origNewServer := ghbridgeNewServerHook
	origLookPath := ghExecLookPathHook
	t.Cleanup(func() {
		ghbridgeRegisterMCPHook = origRegisterMCP
		ghbridgeMCPConfigPathHook = origMCPConfigPath
		ghbridgeNewHook = origNew
		ghbridgeClassifyHook = origClassify
		ghbridgeAllowedSurfaceHook = origAllowedSurface
		ghbridgeNewServerHook = origNewServer
		ghExecLookPathHook = origLookPath
	})
}

func runGhCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewGhCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestGhSetupSuccess(t *testing.T) {
	saveGhHooks(t)
	ghbridgeRegisterMCPHook = func(string) (string, error) { return "/tmp/mcp.json", nil }
	ghbridgeMCPConfigPathHook = func() string { return "/tmp/mcp.json" }
	out, err := runGhCmd(t, "setup")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "gh MCP bridge registered") {
		t.Errorf("expected setup output, got %q", out.String())
	}
}

func TestGhSetupError(t *testing.T) {
	saveGhHooks(t)
	ghbridgeRegisterMCPHook = func(string) (string, error) { return "", errors.New("register boom") }
	ghbridgeMCPConfigPathHook = func() string { return "/tmp/mcp.json" }
	_, err := runGhCmd(t, "setup")
	if err == nil || !strings.Contains(err.Error(), "register boom") {
		t.Fatalf("expected register error, got %v", err)
	}
}

func TestGhDoctorNotFound(t *testing.T) {
	saveGhHooks(t)
	ghExecLookPathHook = func(string) (string, error) { return "", errors.New("not found") }
	_, err := runGhCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "gh not installed") {
		t.Fatalf("expected gh not installed, got %v", err)
	}
}

func TestGhDoctorUnhealthy(t *testing.T) {
	saveGhHooks(t)
	ghExecLookPathHook = func(string) (string, error) { return "/usr/local/bin/gh", nil }
	ghbridgeNewHook = func() *ghbridge.Bridge {
		return ghbridge.NewWithRunner(func(context.Context, []string) (string, string, error) {
			return "", "not logged in", errors.New("auth failed")
		}, 0)
	}
	_, err := runGhCmd(t, "doctor")
	if err == nil || !strings.Contains(err.Error(), "gh unhealthy") {
		t.Fatalf("expected gh unhealthy, got %v", err)
	}
}

func TestGhDoctorHealthy(t *testing.T) {
	saveGhHooks(t)
	ghExecLookPathHook = func(string) (string, error) { return "/usr/local/bin/gh", nil }
	ghbridgeNewHook = func() *ghbridge.Bridge {
		return ghbridge.NewWithRunner(func(context.Context, []string) (string, string, error) {
			return "logged in", "", nil
		}, 0)
	}
	out, err := runGhCmd(t, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "reachable + auth ok") {
		t.Errorf("expected healthy output, got %q", out.String())
	}
}

func TestGhRunClassifyError(t *testing.T) {
	saveGhHooks(t)
	ghbridgeClassifyHook = func([]string) (ghbridge.Tier, error) { return ghbridge.TierForbidden, errors.New("classify boom") }
	_, err := runGhCmd(t, "run", "issue")
	if err == nil || !strings.Contains(err.Error(), "classify boom") {
		t.Fatalf("expected classify error, got %v", err)
	}
}

func TestGhRunForbidden(t *testing.T) {
	saveGhHooks(t)
	ghbridgeClassifyHook = func([]string) (ghbridge.Tier, error) { return ghbridge.TierForbidden, nil }
	_, err := runGhCmd(t, "run", "issue")
	if err == nil || !strings.Contains(err.Error(), "forbidden by ghbridge policy") {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestGhRunMutating(t *testing.T) {
	saveGhHooks(t)
	ghbridgeClassifyHook = func([]string) (ghbridge.Tier, error) { return ghbridge.TierMutating, nil }
	_, err := runGhCmd(t, "run", "issue")
	if err == nil || !strings.Contains(err.Error(), "mutating command") {
		t.Fatalf("expected mutating error, got %v", err)
	}
}

func TestGhRunReadOnlySuccess(t *testing.T) {
	saveGhHooks(t)
	ghbridgeClassifyHook = func([]string) (ghbridge.Tier, error) { return ghbridge.TierReadOnly, nil }
	ghbridgeNewHook = func() *ghbridge.Bridge {
		return ghbridge.NewWithRunner(func(context.Context, []string) (string, string, error) {
			return "issue list\n", "", nil
		}, 0)
	}
	out, err := runGhCmd(t, "run", "issue", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "issue list") {
		t.Errorf("expected issue list output, got %q", out.String())
	}
}

func TestGhRunReadOnlyExecuteError(t *testing.T) {
	saveGhHooks(t)
	ghbridgeClassifyHook = func([]string) (ghbridge.Tier, error) { return ghbridge.TierReadOnly, nil }
	ghbridgeNewHook = func() *ghbridge.Bridge {
		return ghbridge.NewWithRunner(func(context.Context, []string) (string, string, error) {
			return "", "fail", errors.New("exec boom")
		}, 0)
	}
	_, err := runGhCmd(t, "run", "issue", "list")
	if err == nil || (!strings.Contains(err.Error(), "exec boom") && !strings.Contains(err.Error(), "fail")) {
		t.Fatalf("expected exec error, got %v", err)
	}
}

func TestGhRunUnknownTier(t *testing.T) {
	saveGhHooks(t)
	ghbridgeClassifyHook = func([]string) (ghbridge.Tier, error) { return ghbridge.Tier(99), nil }
	_, err := runGhCmd(t, "run", "issue")
	if err == nil || !strings.Contains(err.Error(), "unknown tier") {
		t.Fatalf("expected unknown tier error, got %v", err)
	}
}

func TestGhSurface(t *testing.T) {
	saveGhHooks(t)
	ghbridgeAllowedSurfaceHook = func() string { return "issue, pr, repo" }
	out, err := runGhCmd(t, "surface")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "issue, pr, repo") {
		t.Errorf("expected surface output, got %q", out.String())
	}
}

func TestGhServe(t *testing.T) {
	saveGhHooks(t)
	ghbridgeNewServerHook = func() *ghbridge.Server {
		return ghbridge.NewServerWithIO(strings.NewReader(""), io.Discard, io.Discard)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := NewGhCmd()
	cmd.SetArgs([]string{"serve"})
	setOutAll(cmd, io.Discard)
	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatal(err)
	}
}
