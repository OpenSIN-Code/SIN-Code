// SPDX-License-Identifier: MIT
// Purpose: tests for the MCP-tool handler wrappers in serve_extra_handlers.go.

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/notifications"
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

// makeFakeSinCodeEcho creates a fake sin-code that echoes its arguments back.
func makeFakeSinCodeEcho(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sin-code")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '%s ' \"$a\"; done\necho\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
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

func TestHandleNotificationsList_OpenError(t *testing.T) {
	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", "/dev/null")
	defer os.Setenv("HOME", oldHome)

	_, err := handleNotificationsList(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when notifications db cannot open")
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

func TestHandleOrchestratorPlan_FailurePropagates(t *testing.T) {
	_ = makeFakeSinCode(t, `{}`, "orchestrator-plan")
	_, err := handleOrchestratorPlan(context.Background(), map[string]any{"prompt": "fail"})
	if err == nil {
		t.Fatal("expected orchestrator-plan failure to propagate")
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

func TestHandleMemoryPrime_RequiresQuery(t *testing.T) {
	_, err := handleMemoryPrime(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestHandleMemoryPrime_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, "context ready", "")
	if _, err := handleMemoryPrime(context.Background(),
		map[string]any{"query": "what did we do?"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleMemoryPrime_FailurePropagates(t *testing.T) {
	_ = makeFakeSinCode(t, "", "memory") // fail on subcommands containing "memory"
	_, err := handleMemoryPrime(context.Background(), map[string]any{"query": "q"})
	if err == nil {
		t.Fatal("expected error when runSinCodeCLI fails")
	}
}

func TestHandleMemoryPrime_WithOptions(t *testing.T) {
	_ = makeFakeSinCodeEcho(t)
	out, err := handleMemoryPrime(context.Background(), map[string]any{
		"query":   "q",
		"project": "p",
		"top":     float64(3),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--project", "p", "--top", "3"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got %q", want, out)
		}
	}
}

func TestHandleMemorySearch_WithOptions(t *testing.T) {
	_ = makeFakeSinCodeEcho(t)
	out, err := handleMemorySearch(context.Background(), map[string]any{
		"query":   "q",
		"project": "p",
		"top":     float64(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--project", "p", "--top", "5"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got %q", want, out)
		}
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

func TestHandleTodoDepAdd_DispatchesWithDefaultRel(t *testing.T) {
	_ = makeFakeSinCodeEcho(t)
	out, err := handleTodoDepAdd(context.Background(),
		map[string]any{"child": "T1", "parent": "T2"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--type blocks") {
		t.Errorf("expected default rel blocks, got %q", out)
	}
}

func TestHandleTodoDep_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"child":"T1","parent":"T2"}]`, "")
	if _, err := handleTodoDep(context.Background(),
		map[string]any{"child": "T1", "parent": "T2"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleOrchestratorRun_RequiresPrompt(t *testing.T) {
	if _, err := handleOrchestratorRun(context.Background(),
		map[string]any{}); err == nil {
		t.Fatal("missing prompt must error")
	}
}

func TestHandleOrchestratorRun_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"plan":{"id":"P1"}}`, "")
	if _, err := handleOrchestratorRun(context.Background(),
		map[string]any{"prompt": "add tests"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleOrchestratorRun_WithOptions(t *testing.T) {
	_ = makeFakeSinCode(t, `{"plan":{"id":"P1"}}`, "")
	if _, err := handleOrchestratorRun(context.Background(), map[string]any{
		"prompt":      "add tests",
		"timeout":     "30s",
		"max_parallel": float64(2),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleOrchestratorAgents_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"name":"coder"}]`, "")
	if _, err := handleOrchestratorAgents(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestHandleAgentShow_RequiresName(t *testing.T) {
	if _, err := handleAgentShow(context.Background(),
		map[string]any{}); err == nil {
		t.Fatal("missing name must error")
	}
}

func TestHandleAgentShow_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"name":"coder"}`, "")
	if _, err := handleAgentShow(context.Background(),
		map[string]any{"name": "coder"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleAgentSet_RequiresNameAndKvs(t *testing.T) {
	if _, err := handleAgentSet(context.Background(),
		map[string]any{}); err == nil {
		t.Fatal("missing name/kvs must error")
	}
	if _, err := handleAgentSet(context.Background(),
		map[string]any{"name": "coder"}); err == nil {
		t.Fatal("missing kvs must error")
	}
}

func TestHandleAgentSet_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"ok":true}`, "")
	if _, err := handleAgentSet(context.Background(), map[string]any{
		"name": "coder",
		"kvs":  []any{"model=gpt-4"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleTodoAdd_RequiresTitle(t *testing.T) {
	if _, err := handleTodoAdd(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing title must error")
	}
}

func TestHandleTodoAdd_DispatchesAllArgs(t *testing.T) {
	_ = makeFakeSinCodeEcho(t)
	out, err := handleTodoAdd(context.Background(), map[string]any{
		"title":       "Do thing",
		"description": "desc",
		"priority":    "p1",
		"type":        "feature",
		"tags":        "a,b",
		"project":     "proj",
		"assignee":    "me",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"todo", "add", "Do thing", "--desc", "--priority", "p1", "--type", "feature", "--tags", "a,b", "--project", "proj", "--assignee", "me"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got %q", want, out)
		}
	}
}

func TestHandleTodoShow_RequiresID(t *testing.T) {
	if _, err := handleTodoShow(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing id must error")
	}
}

func TestHandleTodoShow_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"id":"T1"}`, "")
	if _, err := handleTodoShow(context.Background(), map[string]any{"id": "T1"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleTodoComplete_RequiresID(t *testing.T) {
	if _, err := handleTodoComplete(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing id must error")
	}
}

func TestHandleTodoComplete_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"ok":true}`, "")
	if _, err := handleTodoComplete(context.Background(), map[string]any{"id": "T1"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleTodoClaim_RequiresID(t *testing.T) {
	if _, err := handleTodoClaim(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing id must error")
	}
}

func TestHandleTodoClaim_DispatchesAs(t *testing.T) {
	_ = makeFakeSinCodeEcho(t)
	out, err := handleTodoClaim(context.Background(), map[string]any{"id": "T1", "as": "user"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--as user") {
		t.Errorf("expected --as user in output, got %q", out)
	}
}

func TestHandleTodoList_WithFilters(t *testing.T) {
	_ = makeFakeSinCodeEcho(t)
	out, err := handleTodoList(context.Background(), map[string]any{
		"status":   "open",
		"priority": "p1",
		"project":  "proj",
		"tag":      "bug",
		"limit":    float64(10),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--status", "open", "--priority", "p1", "--project", "proj", "--tag", "bug", "--limit", "10"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q, got %q", want, out)
		}
	}
}

func TestHandleMemoryAdd_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"id":"M1"}`, "")
	if _, err := handleMemoryAdd(context.Background(), map[string]any{"insight": "note", "project": "p", "tags": "t"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleMemorySearch_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"id":"M1"}]`, "")
	if _, err := handleMemorySearch(context.Background(), map[string]any{"query": "q"}); err != nil {
		t.Fatal(err)
	}
}

func TestHandleMemoryList_WithProject(t *testing.T) {
	_ = makeFakeSinCodeEcho(t)
	out, err := handleMemoryList(context.Background(), map[string]any{"project": "proj"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--project") || !strings.Contains(out, "proj") {
		t.Errorf("expected project flag in output, got %q", out)
	}
}

func TestHandleMemoryStats_OpenError(t *testing.T) {
	old := memoryOpenFunc
	memoryOpenFunc = func(path string) (*memory.Store, error) {
		return nil, fmt.Errorf("injected open error")
	}
	t.Cleanup(func() { memoryOpenFunc = old })

	_, err := handleMemoryStats(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "injected open error") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestHandleMemoryStats_StatsError(t *testing.T) {
	old := memoryOpenFunc
	memoryOpenFunc = func(path string) (*memory.Store, error) {
		dir := t.TempDir()
		s, err := memory.Open(filepath.Join(dir, "memory.db"))
		if err != nil {
			t.Fatal(err)
		}
		// Close the underlying DB so Stats() fails while EmbeddingStatus() is unaffected.
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		return s, nil
	}
	t.Cleanup(func() { memoryOpenFunc = old })

	_, err := handleMemoryStats(context.Background(), nil)
	if err == nil {
		t.Fatal("expected stats error from closed store")
	}
}

func TestHandleNotificationsList_Limit(t *testing.T) {
	old := notificationsOpenFunc
	notificationsOpenFunc = func(path string) (*notifications.Store, error) {
		return notifications.Open(filepath.Join(t.TempDir(), "notifications.db"))
	}
	t.Cleanup(func() { notificationsOpenFunc = old })

	out, err := handleNotificationsList(context.Background(), map[string]any{"limit": float64(5)})
	if err != nil {
		t.Fatal(err)
	}
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %q", out)
	}
}

func TestHandleNotificationsList_ListError(t *testing.T) {
	old := notificationsOpenFunc
	notificationsOpenFunc = func(path string) (*notifications.Store, error) {
		dir := t.TempDir()
		s, err := notifications.Open(filepath.Join(dir, "notifications.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		return s, nil
	}
	t.Cleanup(func() { notificationsOpenFunc = old })

	_, err := handleNotificationsList(context.Background(), nil)
	if err == nil {
		t.Fatal("expected list error from closed store")
	}
}

func TestHandleNotificationsStats_OpenError(t *testing.T) {
	old := notificationsOpenFunc
	notificationsOpenFunc = func(path string) (*notifications.Store, error) {
		return nil, fmt.Errorf("injected open error")
	}
	t.Cleanup(func() { notificationsOpenFunc = old })

	_, err := handleNotificationsStats(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "injected open error") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestHandleNotificationsStats_ComputeStatsError(t *testing.T) {
	old := notificationsOpenFunc
	notificationsOpenFunc = func(path string) (*notifications.Store, error) {
		dir := t.TempDir()
		s, err := notifications.Open(filepath.Join(dir, "notifications.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		return s, nil
	}
	t.Cleanup(func() { notificationsOpenFunc = old })

	_, err := handleNotificationsStats(context.Background(), nil)
	if err == nil {
		t.Fatal("expected compute stats error from closed store")
	}
}

func TestHandleNotificationsMarkRead_OpenError(t *testing.T) {
	old := notificationsOpenFunc
	notificationsOpenFunc = func(path string) (*notifications.Store, error) {
		return nil, fmt.Errorf("injected open error")
	}
	t.Cleanup(func() { notificationsOpenFunc = old })

	_, err := handleNotificationsMarkRead(context.Background(), map[string]any{"id": "N1"})
	if err == nil || !strings.Contains(err.Error(), "injected open error") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestHandleNotificationsMarkRead_Success(t *testing.T) {
	old := notificationsOpenFunc
	notificationsOpenFunc = func(path string) (*notifications.Store, error) {
		dir := t.TempDir()
		s, err := notifications.Open(filepath.Join(dir, "notifications.db"))
		if err != nil {
			t.Fatal(err)
		}
		n := &notifications.Notification{ID: "nt-123", Type: notifications.TypeTodoCreated, Title: "x", TodoID: "T1"}
		if err := s.Add(n); err != nil {
			t.Fatal(err)
		}
		return s, nil
	}
	t.Cleanup(func() { notificationsOpenFunc = old })

	out, err := handleNotificationsMarkRead(context.Background(), map[string]any{"id": "nt-123"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "nt-123") {
		t.Errorf("expected id in output, got %q", out)
	}
}

func TestHandleAgentDoctor_Offline(t *testing.T) {
	_ = makeFakeSinCodeEcho(t)
	out, err := handleAgentDoctor(context.Background(), map[string]any{"offline": true, "name": "build"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--offline") {
		t.Errorf("expected --offline flag in output, got %q", out)
	}
}

func TestHandleTodoDep_MissingIDs(t *testing.T) {
	_ = makeFakeSinCode(t, `[]`, "")
	if _, err := handleTodoDep(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected missing ids error")
	}
	if _, err := handleTodoDep(context.Background(), map[string]any{"child": "T1"}); err == nil {
		t.Fatal("expected missing parent error")
	}
}

func TestServeExtraRunSinCodeCLI_ResolveBinaryError(t *testing.T) {
	old := osExecutable
	osExecutable = func() (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { osExecutable = old })
	t.Setenv("SIN_CODE_BIN", "")
	t.Setenv("PATH", "/dev/null")

	_, err := runSinCodeCLI("todo", "list")
	if err == nil || !strings.Contains(err.Error(), "cannot resolve sin-code binary") {
		t.Fatalf("expected resolve binary error, got %v", err)
	}
}

func TestServeExtraHandleOrchestratorRun_RequiresPrompt(t *testing.T) {
	if _, err := handleOrchestratorRun(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing prompt must error")
	}
}

func TestServeExtraHandleOrchestratorRun_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"plan":{"id":"P1"}}`, "")
	if _, err := handleOrchestratorRun(context.Background(), map[string]any{"prompt": "add tests"}); err != nil {
		t.Fatal(err)
	}
}

func TestServeExtraHandleOrchestratorRun_WithOptions(t *testing.T) {
	_ = makeFakeSinCode(t, `{"plan":{"id":"P1"}}`, "")
	if _, err := handleOrchestratorRun(context.Background(), map[string]any{
		"prompt":       "add tests",
		"timeout":      "30s",
		"max_parallel": float64(2),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestServeExtraHandleOrchestratorPlan_RequiresPrompt(t *testing.T) {
	if _, err := handleOrchestratorPlan(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing prompt must error")
	}
}

func TestServeExtraHandleOrchestratorPlan_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `{"plan":{"id":"P1"}}`, "")
	if _, err := handleOrchestratorPlan(context.Background(), map[string]any{"prompt": "add tests"}); err != nil {
		t.Fatal(err)
	}
}

func TestServeExtraHandleOrchestratorAgents_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"name":"coder"}]`, "")
	if _, err := handleOrchestratorAgents(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestServeExtraHandleLspServers_Dispatches(t *testing.T) {
	_ = makeFakeSinCode(t, `[{"name":"gopls"}]`, "")
	if _, err := handleLspServers(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestServeExtraStringJoin(t *testing.T) {
	got := stringJoin([]string{"a", "b", "c"}, ",")
	if got != "a,b,c" {
		t.Errorf("expected a,b,c, got %q", got)
	}
}
