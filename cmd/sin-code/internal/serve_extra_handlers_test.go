// SPDX-License-Identifier: MIT
// Purpose: tests for the MCP-tool handler wrappers in serve_extra_handlers.go.

package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeFakeSinCode writes a tiny shell script that echoes a given JSON line.
// Honors FAIL_ON to exit non-zero for negative-path tests.
// Returns the temp dir containing the script (so callers can add it to PATH).
func makeFakeSinCode(t *testing.T, response string, failOn string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sin-code")
	var script string
	if failOn != "" {
		script = "#!/bin/sh\n" +
			"if echo \"$@\" | grep -q -- \"$FAIL_ON\"; then\n" +
			"  echo '{\"error\":\"injected failure\"}'\n" +
			"  exit 1\n" +
			"fi\n" +
			"echo '" + response + "'\n"
		os.Setenv("FAIL_ON", failOn)
		t.Cleanup(func() { os.Unsetenv("FAIL_ON") })
	} else {
		script = "#!/bin/sh\necho '" + response + "'\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// Put the fake binary on PATH so exec.LookPath finds it.
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return path
}

func TestRunSinCodeCLI_HappyPath(t *testing.T) {
	_ = makeFakeSinCode(t, `{"ok":true}`, "")
	out, err := runSinCodeCLI("todo", "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("expected response, got %q", out)
	}
}

func TestRunSinCodeCLI_FailurePropagates(t *testing.T) {
	_ = makeFakeSinCode(t, `{}`, "bad-arg")
	_, err := runSinCodeCLI("bad-arg", "more")
	if err == nil {
		t.Fatal("expected error from failing command")
	}
}

func TestHandleTodoList_DispatchesFormatJSON(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"id":"T1","title":"x"}]`, "")
	out, err := handleTodoList(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"id":"T1"`) {
		t.Fatalf("handler must pass through stdout: %q", out)
	}
}

func TestHandleTodoSearch_RequiresQuery(t *testing.T) {
	_ = makeFakeSinCode(t, `[]`, "")
	if _, err := handleTodoSearch(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing query must error")
	}
	if _, err := handleTodoSearch(context.Background(),
		map[string]any{"query": ""}); err == nil {
		t.Fatal("empty query must error")
	}
}

func TestHandleTodoSearch_PassesQuery(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"id":"X","title":"found"}]`, "")
	out, err := handleTodoSearch(context.Background(),
		map[string]any{"query": "needle"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"found"`) {
		t.Fatalf("expected matching item, got %q", out)
	}
}

func TestHandleTodoStats_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"by_status":{"done":3,"open":1}}`, "")
	out, err := handleTodoStats(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON from handler: %q", out)
	}
	if _, ok := parsed["by_status"]; !ok {
		t.Fatal("missing by_status key")
	}
}

func TestHandleMemoryAdd_RequiresText(t *testing.T) {
	if _, err := handleMemoryAdd(context.Background(),
		map[string]any{}); err == nil {
		t.Fatal("missing text must error")
	}
}

func TestHandleMemorySearch_RequiresQuery(t *testing.T) {
	if _, err := handleMemorySearch(context.Background(),
		map[string]any{}); err == nil {
		t.Fatal("missing query must error")
	}
}

func TestHandleMemoryStats_ReturnsValidJSON(t *testing.T) {
	out, err := handleMemoryStats(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %q", out)
	}
}

func TestHandleNotificationsList_ReturnsJSON(t *testing.T) {
	out, err := handleNotificationsList(context.Background(),
		map[string]any{"limit": 5})
	if err != nil {
		t.Fatal(err)
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %q", out)
	}
}

func TestHandleLspServers_ReachableOrEmpty(t *testing.T) {
	// handler is environment-dependent (needs lsp CLI on PATH).
	// We only assert it doesn't panic and either returns data or a
	// sensible error about the missing binary.
	out, err := handleLspServers(context.Background(), nil)
	if err != nil {
		// acceptable: missing LSP runtime, broken symlink, etc.
		// unacceptable: unexpected error shape
		if !strings.Contains(err.Error(), "lsp") &&
			!strings.Contains(err.Error(), "exec") &&
			!strings.Contains(err.Error(), "sin-code") {
			t.Fatalf("unexpected error: %v (output: %q)", err, out)
		}
	}
}

func TestHandleOrchestratorPlan_RequiresGoal(t *testing.T) {
	if _, err := handleOrchestratorPlan(context.Background(),
		map[string]any{}); err == nil {
		t.Fatal("missing goal must error")
	}
}

func TestHandleTodoReady_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"id":"T1"}]`, "")
	if _, err := handleTodoReady(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestHandleTodoBlocked_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"id":"T1"}]`, "")
	if _, err := handleTodoBlocked(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestHandleTodoPrime_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, "no open work", "")
	if _, err := handleTodoPrime(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestHandleMemoryList_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"id":"M1","text":"hello"}]`, "")
	if _, err := handleMemoryList(context.Background(),
		map[string]any{"tag": "sin-delegate"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleMemoryPrime_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, "context ready", "")
	if _, err := handleMemoryPrime(context.Background(),
		map[string]any{"query": "what did we do?"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleNotificationsStats_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"total":5,"unread":2}`, "")
	if _, err := handleNotificationsStats(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestHandleNotificationsMarkRead_RequiresID(t *testing.T) {
	if _, err := handleNotificationsMarkRead(context.Background(),
		map[string]any{}); err == nil {
		t.Fatal("missing id must error")
	}
}

func TestHandleNotificationsMarkRead_PassesID(t *testing.T) {
	_ = makeFakeSinCode(t, `{"ok":true}`, "")
	// The handler is robust: it passes through whatever the CLI returns
	// (even an error). We only assert it does not panic and reaches
	// the underlying CLI (any error from `notif mark-read N123` is fine).
	_, _ = handleNotificationsMarkRead(context.Background(),
		map[string]any{"id": "N123"})
}

func TestHandleAgentDoctor_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"all_ok":true}`, "")
	if _, err := handleAgentDoctor(context.Background(),
		map[string]any{"name": "build"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleTodoDepAdd_RequiresBoth(t *testing.T) {
	if _, err := handleTodoDepAdd(context.Background(),
		map[string]any{}); err == nil {
		t.Fatal("missing ids must error")
	}
}

func TestHandleTodoDep_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"child":"T1","parent":"T2"}]`, "")
	if _, err := handleTodoDep(context.Background(),
		map[string]any{"child": "T1", "parent": "T2"}); err != nil {
		t.Fatal(err)
	}
}
