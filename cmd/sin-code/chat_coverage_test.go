package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// chatHookSnapshot captures all package-level chat hooks for restoration.
type chatHookSnapshot struct {
	loadAgentFn             func(string) (orchestrator.AgentConfig, string, error)
	newLLMClientFn          func(string, string) *llm.Client
	newProviderCompletionFn func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error)
	rulesForAgentFn         func(orchestrator.AgentConfig) []permission.Rule
	getwdFn                 func() (string, error)
	newHooksFn              func([]hooks.Hook) *hooks.Engine
	loadMCPConfigsFn        func(string) []mcpclient.ServerConfig
	newMCPManagerFn         func([]mcpclient.ServerConfig) *mcpclient.Manager
	mcpConnectAllFn         func(*mcpclient.Manager, context.Context) error
	openSessionFn           func(string) (*session.Store, error)
	newGateFn               func(string, verify.Runner, verify.Runner) *verify.Gate
	askFn                   func(agentloop.ToolCall) bool
	printResultFn           func(*agentloop.Result, bool) error
	runOverrideFn           func(context.Context, *session.Session, string) (*agentloop.Result, error)
	stdout                  io.Writer
	stderr                  io.Writer
	stdin                   io.Reader
}

func snapshotChatHooks() chatHookSnapshot {
	return chatHookSnapshot{
		loadAgentFn:             chatLoadAgentFn,
		newLLMClientFn:          chatNewLLMClientFn,
		newProviderCompletionFn: chatNewProviderCompletionFn,
		rulesForAgentFn:         chatRulesForAgentFn,
		getwdFn:                 chatGetwdFn,
		newHooksFn:              chatNewHooksFn,
		loadMCPConfigsFn:        chatLoadMCPConfigsFn,
		newMCPManagerFn:         chatNewMCPManagerFn,
		mcpConnectAllFn:         chatMCPConnectAllFn,
		openSessionFn:           chatOpenSessionFn,
		newGateFn:               chatNewGateFn,
		askFn:                   chatAskFn,
		printResultFn:           chatPrintResultFn,
		runOverrideFn:           chatRunOverrideFn,
		stdout:                  chatStdout,
		stderr:                  chatStderr,
		stdin:                   chatStdin,
	}
}

func restoreChatHooks(s chatHookSnapshot) {
	chatLoadAgentFn = s.loadAgentFn
	chatNewLLMClientFn = s.newLLMClientFn
	chatNewProviderCompletionFn = s.newProviderCompletionFn
	chatRulesForAgentFn = s.rulesForAgentFn
	chatGetwdFn = s.getwdFn
	chatNewHooksFn = s.newHooksFn
	chatLoadMCPConfigsFn = s.loadMCPConfigsFn
	chatNewMCPManagerFn = s.newMCPManagerFn
	chatMCPConnectAllFn = s.mcpConnectAllFn
	chatOpenSessionFn = s.openSessionFn
	chatNewGateFn = s.newGateFn
	chatAskFn = s.askFn
	chatPrintResultFn = s.printResultFn
	chatRunOverrideFn = s.runOverrideFn
	chatStdout = s.stdout
	chatStderr = s.stderr
	chatStdin = s.stdin
}

func patchChatHooks(t *testing.T) chatHookSnapshot {
	s := snapshotChatHooks()
	t.Cleanup(func() { restoreChatHooks(s) })
	return s
}

func fakeSessionStore(t *testing.T) *session.Store {
	t.Helper()
	db := filepath.Join(t.TempDir(), "sessions.db")
	store, err := session.Open(db)
	if err != nil {
		t.Fatalf("open session store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "a", "b"); got != "a" {
		t.Errorf("firstNonEmpty = %q, want a", got)
	}
	if got := firstNonEmpty("", "", ""); got != "" {
		t.Errorf("firstNonEmpty = %q, want empty", got)
	}
	if got := firstNonEmpty("x"); got != "x" {
		t.Errorf("firstNonEmpty = %q, want x", got)
	}
}

func TestCommandRunner(t *testing.T) {
	if got := commandRunner(""); got != nil {
		t.Errorf("commandRunner(\"\") = %v, want nil", got)
	}
	ok, report, err := commandRunner("echo hello")(context.Background(), ".")
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if !ok {
		t.Error("expected success for echo")
	}
	if !strings.Contains(report, "hello") {
		t.Errorf("report = %q, want hello", report)
	}

	ok, report, err = commandRunner("echo failure; exit 1")(context.Background(), ".")
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if ok {
		t.Error("expected failure for false")
	}
	if !strings.Contains(report, "failure") {
		t.Errorf("expected non-empty report for failing command, got %q", report)
	}
}

func TestLoadHooks(t *testing.T) {
	ws := t.TempDir()
	// No files: returns empty.
	if got := loadHooks(ws); len(got) != 0 {
		t.Errorf("loadHooks no files = %d, want 0", len(got))
	}

	// Invalid JSON file in workspace.
	sinDir := filepath.Join(ws, ".sin-code")
	os.MkdirAll(sinDir, 0o755)
	invalidPath := filepath.Join(sinDir, "hooks.json")
	os.WriteFile(invalidPath, []byte("not json"), 0o644)
	if got := loadHooks(ws); len(got) != 0 {
		t.Errorf("loadHooks invalid json = %d, want 0", len(got))
	}

	// Valid JSON file.
	validPath := filepath.Join(sinDir, "hooks.json")
	os.WriteFile(validPath, []byte(`[{"event":"tool.pre","type":"prompt","text":"hi"}]`), 0o644)
	got := loadHooks(ws)
	if len(got) != 1 || got[0].Event != "tool.pre" {
		t.Errorf("loadHooks valid = %+v, want 1 tool.pre hook", got)
	}
}

func TestPrintResult(t *testing.T) {
	patchChatHooks(t)
	var buf bytes.Buffer
	chatStdout = &buf

	res := &agentloop.Result{SessionID: "s1", Summary: "done", Verified: true, Turns: 3}
	if err := printResult(res, false); err != nil {
		t.Fatalf("printResult text: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "done") || !strings.Contains(out, "[session=s1 verified=true turns=3]") {
		t.Errorf("text output = %q", out)
	}

	buf.Reset()
	if err := printResult(res, true); err != nil {
		t.Fatalf("printResult json: %v", err)
	}
	if !strings.Contains(buf.String(), `"session_id": "s1"`) {
		t.Errorf("json output = %q", buf.String())
	}
}

func TestTerminalAsk(t *testing.T) {
	patchChatHooks(t)
	chatStdin = strings.NewReader("y\n")
	if !terminalAsk(agentloop.ToolCall{Name: "x"}) {
		t.Error("terminalAsk y = false")
	}
	chatStdin = strings.NewReader("no\n")
	if terminalAsk(agentloop.ToolCall{Name: "x"}) {
		t.Error("terminalAsk no = true")
	}
	chatStdin = strings.NewReader("")
	if terminalAsk(agentloop.ToolCall{Name: "x"}) {
		t.Error("terminalAsk EOF = true")
	}
}

func TestRunChat_AgentLoadError(t *testing.T) {
	patchChatHooks(t)
	chatLoadAgentFn = func(string) (orchestrator.AgentConfig, string, error) {
		return orchestrator.AgentConfig{}, "", errors.New("agent fail")
	}
	err := runChat(context.Background(), &chatOptions{agent: "bad"})
	if err == nil || !strings.Contains(err.Error(), "agent fail") {
		t.Errorf("expected agent load error, got %v", err)
	}
}

func TestRunChat_GetwdError(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return "", errors.New("getwd fail") }
	err := runChat(context.Background(), &chatOptions{})
	if err == nil || !strings.Contains(err.Error(), "getwd fail") {
		t.Errorf("expected getwd error, got %v", err)
	}
}

func TestRunChat_MCPConnectError(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager {
		return mcpclient.NewManager(nil)
	}
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return errors.New("mcp fail") }
	err := runChat(context.Background(), &chatOptions{})
	if err == nil || !strings.Contains(err.Error(), "mcp fail") {
		t.Errorf("expected mcp error, got %v", err)
	}
}

func TestRunChat_SessionOpenError(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return nil, errors.New("open fail") }
	err := runChat(context.Background(), &chatOptions{})
	if err == nil || !strings.Contains(err.Error(), "open fail") {
		t.Errorf("expected session open error, got %v", err)
	}
}

func TestRunChat_StartOrResumeError(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	store := fakeSessionStore(t)
	chatOpenSessionFn = func(string) (*session.Store, error) { return store, nil }
	err := runChat(context.Background(), &chatOptions{resume: "nonexistent-session-id"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected start/resume error, got %v", err)
	}
}

func TestRunChat_HeadlessSuccess(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
			return nil, errors.New("should not be called")
		}
	}
	chatRunOverrideFn = func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{SessionID: "s1", Summary: "ok", Verified: true, Turns: 1}, nil
	}
	var called bool
	chatPrintResultFn = func(res *agentloop.Result, jsonOut bool) error {
		called = true
		if !jsonOut || res.SessionID != "s1" {
			return errors.New("unexpected result")
		}
		return nil
	}
	err := runChat(context.Background(), &chatOptions{prompt: "hi", jsonOut: true})
	if err != nil {
		t.Fatalf("runChat: %v", err)
	}
	if !called {
		t.Error("printResult was not called")
	}
}

func TestRunChat_HeadlessRunError(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatRunOverrideFn = func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return nil, errors.New("run fail")
	}
	err := runChat(context.Background(), &chatOptions{prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "run fail") {
		t.Errorf("expected run error, got %v", err)
	}
}

func TestRunChat_REPLExit(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatStdin = strings.NewReader("exit\n")
	err := runChat(context.Background(), &chatOptions{})
	if err != nil {
		t.Fatalf("runChat exit: %v", err)
	}
}

func TestRunChat_REPLQuit(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatStdin = strings.NewReader("quit\n")
	err := runChat(context.Background(), &chatOptions{})
	if err != nil {
		t.Fatalf("runChat quit: %v", err)
	}
}

func TestRunChat_REPLEmptyAndSuccess(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatRunOverrideFn = func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{SessionID: "s1", Summary: "ok", Verified: true, Turns: 1}, nil
	}
	var buf bytes.Buffer
	chatStderr = &buf
	chatStdout = io.Discard
	chatStdin = strings.NewReader("\n\nhello\nexit\n")
	err := runChat(context.Background(), &chatOptions{})
	if err != nil {
		t.Fatalf("runChat repl success: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("unexpected stderr: %s", buf.String())
	}
}

func TestRunChat_REPLRunError(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatRunOverrideFn = func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return nil, errors.New("run error")
	}
	var buf bytes.Buffer
	chatStderr = &buf
	chatStdout = io.Discard
	chatStdin = strings.NewReader("prompt\nexit\n")
	err := runChat(context.Background(), &chatOptions{})
	if err != nil {
		t.Fatalf("runChat repl error: %v", err)
	}
	if !strings.Contains(buf.String(), "run error") {
		t.Errorf("stderr = %q, want run error", buf.String())
	}
}

func TestRunChat_YoloAndVerifyMode(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatRunOverrideFn = func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{SessionID: "s1", Summary: "ok", Verified: true, Turns: 1}, nil
	}
	chatStdin = strings.NewReader("exit\n")
	chatAskFn = func(tc agentloop.ToolCall) bool { return false }
	err := runChat(context.Background(), &chatOptions{yolo: true, verifyMode: "off", maxTurns: 1})
	if err != nil {
		t.Fatalf("runChat yolo: %v", err)
	}
}

func TestRunChat_ResumeSession(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	store := fakeSessionStore(t)
	sess, _ := store.StartOrResume("")
	chatOpenSessionFn = func(string) (*session.Store, error) { return store, nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatRunOverrideFn = func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{SessionID: sess.ID, Summary: "ok", Verified: false, Turns: 1}, nil
	}
	chatStdin = strings.NewReader("exit\n")
	err := runChat(context.Background(), &chatOptions{resume: sess.ID})
	if err != nil {
		t.Fatalf("runChat resume: %v", err)
	}
}

func TestBuiltinSpecs(t *testing.T) {
	specs := builtinSpecs()
	names := map[string]bool{}
	for _, s := range specs {
		names[s.Name] = true
	}
	want := []string{"sin_read", "sin_write", "sin_edit", "sin_bash", "sin_search", "sin_bootstrap_skill", "sin_git_log", "sin_git_diff", "sin_git_commit", "sin_http_get", "sin_test"}
	for _, w := range want {
		if !names[w] {
			t.Errorf("builtinSpecs missing %s", w)
		}
	}
}

func TestBuiltinTool(t *testing.T) {
	patchToolHooks(t)
	ctx := context.Background()

	toolReadFn = func(string) (string, error) { return "read", nil }
	if out, _ := builtinTool(ctx, "", "sin_read", map[string]any{"path": "p"}); out != "read" {
		t.Errorf("sin_read = %q", out)
	}
	toolWriteFn = func(string, string) (string, error) { return "write", nil }
	if out, _ := builtinTool(ctx, "", "sin_write", map[string]any{"path": "p", "content": "c"}); out != "write" {
		t.Errorf("sin_write = %q", out)
	}
	toolEditFn = func(string, string, string) (string, error) { return "edit", nil }
	if out, _ := builtinTool(ctx, "", "sin_edit", map[string]any{"path": "p", "old": "o", "new": "n"}); out != "edit" {
		t.Errorf("sin_edit = %q", out)
	}
	toolBashFn = func(context.Context, string) (string, error) { return "bash", nil }
	if out, _ := builtinTool(ctx, "", "sin_bash", map[string]any{"command": "c"}); out != "bash" {
		t.Errorf("sin_bash = %q", out)
	}
	toolSearchFn = func(string, string) (string, error) { return "search", nil }
	if out, _ := builtinTool(ctx, "", "sin_search", map[string]any{"pattern": "p"}); out != "search" {
		t.Errorf("sin_search = %q", out)
	}
	toolBootstrapSkillFn = func(context.Context, string, map[string]any) (string, error) { return "bootstrap", nil }
	if out, _ := builtinTool(ctx, "", "sin_bootstrap_skill", map[string]any{"name": "n"}); out != "bootstrap" {
		t.Errorf("sin_bootstrap_skill = %q", out)
	}
	extraToolFn = func(context.Context, string, map[string]any) (string, error) { return "extra", nil }
	if out, _ := builtinTool(ctx, "", "sin_unknown", map[string]any{}); out != "extra" {
		t.Errorf("unknown = %q", out)
	}
}

func patchToolHooks(t *testing.T) {
	orig := map[string]any{
		"toolReadFn":           toolReadFn,
		"toolWriteFn":          toolWriteFn,
		"toolEditFn":           toolEditFn,
		"toolBashFn":           toolBashFn,
		"toolSearchFn":         toolSearchFn,
		"toolBootstrapSkillFn": toolBootstrapSkillFn,
		"toolSearchWalkErrFn":  toolSearchWalkErrFn,
		"extraToolFn":          extraToolFn,
		"runGitFn":             runGitFn,
		"toolHTTPGetFn":        toolHTTPGetFn,
		"toolTestFn":           toolTestFn,
		"toolHTTPNewRequestFn": toolHTTPNewRequestFn,
		"toolHTTPClientDoFn":   toolHTTPClientDoFn,
		"toolTestRunFn":        toolTestRunFn,
	}
	t.Cleanup(func() {
		toolReadFn = orig["toolReadFn"].(func(string) (string, error))
		toolWriteFn = orig["toolWriteFn"].(func(string, string) (string, error))
		toolEditFn = orig["toolEditFn"].(func(string, string, string) (string, error))
		toolBashFn = orig["toolBashFn"].(func(context.Context, string) (string, error))
		toolSearchFn = orig["toolSearchFn"].(func(string, string) (string, error))
		toolBootstrapSkillFn = orig["toolBootstrapSkillFn"].(func(context.Context, string, map[string]any) (string, error))
		toolSearchWalkErrFn = orig["toolSearchWalkErrFn"].(func(string, error) error)
		extraToolFn = orig["extraToolFn"].(func(context.Context, string, map[string]any) (string, error))
		runGitFn = orig["runGitFn"].(func(context.Context, ...string) (string, error))
		toolHTTPGetFn = orig["toolHTTPGetFn"].(func(context.Context, string) (string, error))
		toolTestFn = orig["toolTestFn"].(func(context.Context, string) (string, error))
		toolHTTPNewRequestFn = orig["toolHTTPNewRequestFn"].(func(context.Context, string, string, io.Reader) (*http.Request, error))
		toolHTTPClientDoFn = orig["toolHTTPClientDoFn"].(func(*http.Request) (*http.Response, error))
		toolTestRunFn = orig["toolTestRunFn"].(func(*exec.Cmd) ([]byte, error))
	})
}

func TestToolRead(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, []byte("hello"), 0o644)
	out, err := toolRead(p)
	if err != nil || out != "hello" {
		t.Errorf("toolRead = %q, %v", out, err)
	}
	if _, err := toolRead(""); err == nil {
		t.Error("toolRead empty path should error")
	}
}

func TestToolWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "a.txt")
	out, err := toolWrite(p, "data")
	if err != nil {
		t.Fatalf("toolWrite: %v", err)
	}
	if !strings.Contains(out, "wrote 4 bytes") {
		t.Errorf("toolWrite output = %q", out)
	}
	if data, _ := os.ReadFile(p); string(data) != "data" {
		t.Errorf("file content = %q", data)
	}
	if _, err := toolWrite("", "x"); err == nil {
		t.Error("toolWrite empty path should error")
	}
}

func TestToolEdit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	os.WriteFile(p, []byte("old text"), 0o644)
	out, err := toolEdit(p, "old", "new")
	if err != nil {
		t.Fatalf("toolEdit: %v", err)
	}
	if data, _ := os.ReadFile(p); string(data) != "new text" {
		t.Errorf("edit result = %q", data)
	}
	if !strings.Contains(out, "edited") {
		t.Errorf("toolEdit output = %q", out)
	}
	if _, err := toolEdit("", "old", "new"); err == nil {
		t.Error("toolEdit empty path should error")
	}
	if _, err := toolEdit(p, "missing", "x"); err == nil {
		t.Error("toolEdit missing old should error")
	}
}

func TestToolBash(t *testing.T) {
	out, err := toolBash(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("toolBash: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("toolBash = %q", out)
	}
	if _, err := toolBash(context.Background(), ""); err == nil {
		t.Error("toolBash empty command should error")
	}
}

func TestToolSearch(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle haystack"), 0o644)
	out, err := toolSearch("needle", dir)
	if err != nil {
		t.Fatalf("toolSearch: %v", err)
	}
	if !strings.Contains(out, "needle") {
		t.Errorf("toolSearch = %q", out)
	}
	if _, err := toolSearch("", dir); err == nil {
		t.Error("toolSearch empty pattern should error")
	}
}

func TestToolBootstrapSkill(t *testing.T) {
	if os.Getenv("SIN_ALLOW_BOOTSTRAP") != "1" {
		t.Setenv("SIN_ALLOW_BOOTSTRAP", "1")
	}
	_, err := toolBootstrapSkill(context.Background(), t.TempDir(), map[string]any{"name": "bad name"})
	if err == nil {
		t.Error("toolBootstrapSkill invalid name should error")
	}
}

func TestExtraTool(t *testing.T) {
	patchToolHooks(t)
	ctx := context.Background()

	runGitFn = func(context.Context, ...string) (string, error) { return "git", nil }
	if out, _ := extraTool(ctx, "sin_git_log", map[string]any{}); out != "git" {
		t.Errorf("sin_git_log = %q", out)
	}
	if out, _ := extraTool(ctx, "sin_git_diff", map[string]any{"ref": "main"}); out != "git" {
		t.Errorf("sin_git_diff = %q", out)
	}
	if out, _ := extraTool(ctx, "sin_git_diff", map[string]any{}); out != "git" {
		t.Errorf("sin_git_diff no ref = %q", out)
	}
	if out, _ := extraTool(ctx, "sin_git_commit", map[string]any{"message": "m"}); out != "git" {
		t.Errorf("sin_git_commit = %q", out)
	}
	toolHTTPGetFn = func(context.Context, string) (string, error) { return "http", nil }
	if out, _ := extraTool(ctx, "sin_http_get", map[string]any{"url": "http://x"}); out != "http" {
		t.Errorf("sin_http_get = %q", out)
	}
	// Restore the production HTTP getter so the URL validation is exercised for non-http URLs.
	toolHTTPGetFn = toolHTTPGet
	if _, err := extraTool(ctx, "sin_http_get", map[string]any{"url": "ftp://x"}); err == nil {
		t.Error("sin_http_get non-http should error")
	}
	toolTestFn = func(context.Context, string) (string, error) { return "test", nil }
	if out, _ := extraTool(ctx, "sin_test", map[string]any{}); out != "test" {
		t.Errorf("sin_test = %q", out)
	}
	if _, err := extraTool(ctx, "sin_unknown", map[string]any{}); err == nil {
		t.Error("unknown tool should error")
	}
	if _, err := extraTool(ctx, "sin_git_commit", map[string]any{}); err == nil {
		t.Error("sin_git_commit without message should error")
	}
}

func TestRunGit(t *testing.T) {
	out, err := runGit(context.Background(), "version")
	if err != nil {
		t.Fatalf("runGit: %v", err)
	}
	if !strings.Contains(out, "git version") && !strings.Contains(out, "git error") {
		t.Errorf("runGit = %q", out)
	}
}

func TestToolHTTPGet(t *testing.T) {
	if _, err := toolHTTPGet(context.Background(), "ftp://x"); err == nil {
		t.Error("ftp URL should be rejected")
	}
}

func TestToolTest(t *testing.T) {
	if _, err := toolTest(context.Background(), ""); err == nil {
		t.Error("toolTest with no test setup should error")
	}
}

func TestCombinedSpecs(t *testing.T) {
	mgr := mcpclient.NewManager(nil)
	orig := mcpManagerToolsFn
	defer func() { mcpManagerToolsFn = orig }()
	mcpManagerToolsFn = func(*mcpclient.Manager) []mcpclient.Tool {
		return []mcpclient.Tool{
			{Name: "t1", Server: "srv", Qualified: "srv__t1", Description: "d1", InputSchema: nil},
			{Name: "t2", Server: "srv", Qualified: "srv__t2", Description: "", InputSchema: map[string]any{"type": "object"}},
		}
	}
	specs := combinedSpecs(mgr)
	names := map[string]bool{}
	for _, s := range specs {
		names[s.Name] = true
	}
	if !names["srv__t1"] || !names["srv__t2"] {
		t.Errorf("combinedSpecs missing external tools: %+v", names)
	}
	if !names["sin_read"] {
		t.Error("combinedSpecs missing builtin tools")
	}
}

func TestCombinedTool(t *testing.T) {
	mgr := mcpclient.NewManager(nil)
	orig := mcpManagerCallFn
	defer func() { mcpManagerCallFn = orig }()

	mcpManagerCallFn = func(*mcpclient.Manager, context.Context, string, map[string]any) (string, error) {
		return "external", nil
	}
	fn := combinedTool("ws", mgr)
	out, err := fn(context.Background(), "srv__tool", map[string]any{})
	if err != nil || out != "external" {
		t.Errorf("combinedTool external = %q, %v", out, err)
	}

	mcpManagerCallFn = func(*mcpclient.Manager, context.Context, string, map[string]any) (string, error) {
		return "", errors.New("external fail")
	}
	out, err = fn(context.Background(), "srv__tool", map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "external fail") {
		t.Errorf("combinedTool external error = %q, %v", out, err)
	}
}

func TestCombinedTool_Builtin(t *testing.T) {
	patchToolHooks(t)
	mgr := mcpclient.NewManager(nil)
	fn := combinedTool("ws", mgr)
	extraToolFn = func(context.Context, string, map[string]any) (string, error) { return "builtin-extra", nil }
	out, err := fn(context.Background(), "sin_unknown", map[string]any{})
	if err != nil || out != "builtin-extra" {
		t.Errorf("combinedTool builtin = %q, %v", out, err)
	}
}

func TestNewChatCmd(t *testing.T) {
	cmd := NewChatCmd()
	if cmd.Name() != "chat" {
		t.Errorf("command name = %q", cmd.Name())
	}
	if err := cmd.Flags().Set("prompt", "hi"); err != nil {
		t.Fatalf("set prompt: %v", err)
	}
	if err := cmd.Flags().Set("json", "true"); err != nil {
		t.Fatalf("set json: %v", err)
	}
}

func TestRunChat_AgentLoadSuccess(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatLoadAgentFn = func(string) (orchestrator.AgentConfig, string, error) {
		return orchestrator.AgentConfig{Model: "m", BaseURL: "http://localhost", MaxTokens: 100, Temperature: 0.5}, "", nil
	}
	chatStdin = strings.NewReader("exit\n")
	if err := runChat(context.Background(), &chatOptions{agent: "default"}); err != nil {
		t.Fatalf("runChat agent success: %v", err)
	}
}

func TestRunChat_VerifyModeDefault(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatRunOverrideFn = func(context.Context, *session.Session, string) (*agentloop.Result, error) {
		return &agentloop.Result{SessionID: "s1", Summary: "ok", Verified: true, Turns: 1}, nil
	}
	if err := runChat(context.Background(), &chatOptions{prompt: "hi", verifyCmd: "true"}); err != nil {
		t.Fatalf("runChat verify default: %v", err)
	}
}

type errReader struct {
	err error
}

func (r *errReader) Read(p []byte) (int, error) {
	return 0, r.err
}

func TestRunChat_ScannerError(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return t.TempDir(), nil }
	chatNewMCPManagerFn = func([]mcpclient.ServerConfig) *mcpclient.Manager { return mcpclient.NewManager(nil) }
	chatMCPConnectAllFn = func(*mcpclient.Manager, context.Context) error { return nil }
	chatOpenSessionFn = func(string) (*session.Store, error) { return fakeSessionStore(t), nil }
	chatNewLLMClientFn = func(string, string) *llm.Client { return &llm.Client{} }
	chatNewProviderCompletionFn = func(*llm.Client, string, int, float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return nil
	}
	chatStdin = &errReader{err: errors.New("scanner fail")}
	chatStdout = io.Discard
	chatStderr = io.Discard
	err := runChat(context.Background(), &chatOptions{})
	if err == nil || !strings.Contains(err.Error(), "scanner fail") {
		t.Errorf("expected scanner error, got %v", err)
	}
}

func TestNewChatCmd_RunE(t *testing.T) {
	patchChatHooks(t)
	chatGetwdFn = func() (string, error) { return "", errors.New("fail") }
	cmd := NewChatCmd()
	cmd.SetContext(context.Background())
	if err := cmd.RunE(cmd, []string{}); err == nil || !strings.Contains(err.Error(), "fail") {
		t.Errorf("expected runE error, got %v", err)
	}
}

func TestChatMCPConnectAllFnDefault(t *testing.T) {
	orig := chatMCPConnectAllFn
	t.Cleanup(func() { chatMCPConnectAllFn = orig })
	mgr := mcpclient.NewManager(nil)
	if err := chatMCPConnectAllFn(mgr, context.Background()); err != nil {
		t.Fatalf("default connect all: %v", err)
	}
}

func TestMCPManagerCallFnDefault(t *testing.T) {
	orig := mcpManagerCallFn
	t.Cleanup(func() { mcpManagerCallFn = orig })
	mgr := mcpclient.NewManager(nil)
	_, err := mcpManagerCallFn(mgr, context.Background(), "srv__tool", nil)
	if err == nil || !strings.Contains(err.Error(), "no MCP session") {
		t.Errorf("expected no session error, got %v", err)
	}
}

func TestToolBootstrapSkill_EnvNotSet(t *testing.T) {
	t.Setenv("SIN_ALLOW_BOOTSTRAP", "")
	if _, err := toolBootstrapSkill(context.Background(), t.TempDir(), map[string]any{"name": "valid"}); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Errorf("expected env refusal, got %v", err)
	}
}

func TestToolBootstrapSkill_InvalidName(t *testing.T) {
	t.Setenv("SIN_ALLOW_BOOTSTRAP", "1")
	if _, err := toolBootstrapSkill(context.Background(), t.TempDir(), map[string]any{"name": "bad name"}); err == nil || !strings.Contains(err.Error(), "invalid name") {
		t.Errorf("expected invalid name error, got %v", err)
	}
}

func TestToolBootstrapSkill_Success(t *testing.T) {
	patchToolHooks(t)
	t.Setenv("SIN_ALLOW_BOOTSTRAP", "1")
	metaBootstrapSkillFn = func(context.Context, string, string, string) (string, error) {
		return "test__*", nil
	}
	out, err := toolBootstrapSkill(context.Background(), t.TempDir(), map[string]any{"name": "test", "spec": "spec"})
	if err != nil {
		t.Fatalf("toolBootstrapSkill success: %v", err)
	}
	if !strings.Contains(out, "test__*") {
		t.Errorf("output = %q", out)
	}
}

func TestToolBootstrapSkill_MetaError(t *testing.T) {
	patchToolHooks(t)
	t.Setenv("SIN_ALLOW_BOOTSTRAP", "1")
	metaBootstrapSkillFn = func(context.Context, string, string, string) (string, error) {
		return "", errors.New("meta fail")
	}
	if _, err := toolBootstrapSkill(context.Background(), t.TempDir(), map[string]any{"name": "test"}); err == nil || !strings.Contains(err.Error(), "meta fail") {
		t.Errorf("expected meta error, got %v", err)
	}
}

func TestToolRead_ErrorAndTruncation(t *testing.T) {
	dir := t.TempDir()
	if _, err := toolRead(filepath.Join(dir, "missing")); err == nil {
		t.Error("toolRead missing file should error")
	}
	big := strings.Repeat("a", maxReadBytes+10)
	p := filepath.Join(dir, "big.txt")
	os.WriteFile(p, []byte(big), 0o644)
	out, err := toolRead(p)
	if err != nil {
		t.Fatalf("toolRead big: %v", err)
	}
	if !strings.HasSuffix(out, "\n[... truncated]") {
		t.Errorf("expected truncation suffix, got %q", out)
	}
	if len(out) != maxReadBytes+len("\n[... truncated]") {
		t.Errorf("truncated length = %d, want %d", len(out), maxReadBytes+len("\n[... truncated]"))
	}
}

func TestToolWrite_Errors(t *testing.T) {
	dir := t.TempDir()
	parentFile := filepath.Join(dir, "parent")
	os.WriteFile(parentFile, []byte("x"), 0o644)
	if _, err := toolWrite(filepath.Join(parentFile, "child"), "x"); err == nil {
		t.Error("toolWrite MkdirAll on file should error")
	}
	if _, err := toolWrite(dir, "x"); err == nil {
		t.Error("toolWrite to dir should error")
	}
	targetDir := filepath.Join(dir, "targetdir")
	os.MkdirAll(targetDir, 0o755)
	if _, err := toolWrite(targetDir, "x"); err == nil {
		t.Error("toolWrite rename to dir should error")
	}
}

func TestToolWrite_WriteError(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir, 0o755)
	ro := filepath.Join(dir, "ro")
	os.MkdirAll(ro, 0o755)
	os.Chmod(ro, 0o555)
	defer os.Chmod(ro, 0o755)
	if _, err := toolWrite(filepath.Join(ro, "out.txt"), "x"); err == nil {
		t.Error("toolWrite to read-only dir should error")
	}
}

func TestToolEdit_Errors(t *testing.T) {
	dir := t.TempDir()
	if _, err := toolEdit(filepath.Join(dir, "missing"), "old", "new"); err == nil {
		t.Error("toolEdit missing file should error")
	}
	p := filepath.Join(dir, "ro.txt")
	os.WriteFile(p, []byte("old text"), 0o644)
	os.Chmod(p, 0o444)
	t.Cleanup(func() { os.Chmod(p, 0o644) })
	if _, err := toolEdit(p, "old", "new"); err == nil {
		t.Error("toolEdit read-only file should error")
	}
}

func TestToolBash_TruncationAndError(t *testing.T) {
	out, err := toolBash(context.Background(), "yes | head -c 33000")
	if err != nil {
		t.Fatalf("toolBash truncation: %v", err)
	}
	if !strings.HasSuffix(out, "\n[... truncated]") {
		t.Errorf("expected truncation suffix, got %q", out)
	}
	out, err = toolBash(context.Background(), "exit 1")
	if err != nil {
		t.Fatalf("toolBash error: %v", err)
	}
	if !strings.Contains(out, "exit error") {
		t.Errorf("expected error output, got %q", out)
	}
}

func TestToolSearch_Branches(t *testing.T) {
	dir := t.TempDir()

	if out, err := toolSearch("zzzz", dir); err != nil || out != "no matches" {
		t.Errorf("no matches = %q, %v", out, err)
	}

	oldwd, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(oldwd) })
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("needle"), 0o644)
	if out, err := toolSearch("needle", ""); err != nil || !strings.Contains(out, "a.txt") {
		t.Errorf("default dir = %q, %v", out, err)
	}
	os.Chdir(oldwd)

	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "secret"), []byte("needle"), 0o644)
	os.MkdirAll(filepath.Join(dir, "node_modules"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "secret"), []byte("needle"), 0o644)
	os.MkdirAll(filepath.Join(dir, "vendor"), 0o755)
	os.WriteFile(filepath.Join(dir, "vendor", "secret"), []byte("needle"), 0o644)
	if out, err := toolSearch("needle", dir); err != nil || strings.Contains(out, ".git") || strings.Contains(out, "node_modules") || strings.Contains(out, "vendor") {
		t.Errorf("skip dirs = %q", out)
	}

	for i := 0; i < 110; i++ {
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), []byte("needle"), 0o644)
	}
	if out, err := toolSearch("needle", dir); err != nil {
		t.Fatalf("max hits: %v", err)
	} else if cnt := strings.Count(out, "needle"); cnt > maxSearchHits {
		t.Errorf("hits = %d, want <= %d", cnt, maxSearchHits)
	}

	unreadable := filepath.Join(dir, "unreadable")
	os.MkdirAll(unreadable, 0o755)
	os.WriteFile(filepath.Join(unreadable, "x.txt"), []byte("needle"), 0o644)
	os.Chmod(unreadable, 0o000)
	t.Cleanup(func() { os.Chmod(unreadable, 0o755) })
	if _, err := toolSearch("needle", dir); err != nil {
		t.Fatalf("unreadable dir: %v", err)
	}

	toolSearchWalkErrFn = func(path string, err error) error { return err }
	if _, err := toolSearch("needle", dir); err == nil {
		t.Error("expected walk error from hook")
	}
	toolSearchWalkErrFn = func(path string, err error) error { return nil }
	os.Chmod(unreadable, 0o755)

	secret := filepath.Join(dir, "secret.txt")
	os.WriteFile(secret, []byte("secret-needle"), 0o644)
	os.Chmod(secret, 0o000)
	t.Cleanup(func() { os.Chmod(secret, 0o644) })
	if out, err := toolSearch("secret-needle", dir); err != nil || out != "no matches" {
		t.Errorf("unreadable file = %q, %v", out, err)
	}
	os.Chmod(secret, 0o644)

	bigp := filepath.Join(dir, "big.txt")
	os.WriteFile(bigp, []byte(strings.Repeat("big-needle", 2*1024*1024/10+100)), 0o644)
	if out, err := toolSearch("big-needle", dir); err != nil || out != "no matches" {
		t.Errorf("large file = %q, %v", out, err)
	}

	innerDir := t.TempDir()
	many := filepath.Join(innerDir, "many.txt")
	lines := make([]string, 101)
	for i := range lines {
		lines[i] = "many-needle"
	}
	os.WriteFile(many, []byte(strings.Join(lines, "\n")), 0o644)
	if out, err := toolSearch("many-needle", innerDir); err != nil {
		t.Fatalf("inner max hits: %v", err)
	} else if cnt := strings.Count(out, "many.txt"); cnt != maxSearchHits {
		t.Errorf("inner hits = %d, want %d", cnt, maxSearchHits)
	}
}

func TestExtraTool_GitPathAndRef(t *testing.T) {
	patchToolHooks(t)
	ctx := context.Background()
	runGitFn = func(_ context.Context, args ...string) (string, error) {
		return strings.Join(args, " "), nil
	}
	if out, _ := extraTool(ctx, "sin_git_log", map[string]any{"limit": "5", "path": "p"}); out != "log --oneline --decorate -n 5 -- p" {
		t.Errorf("git log path = %q", out)
	}
	if out, _ := extraTool(ctx, "sin_git_diff", map[string]any{"ref": "main"}); out != "diff main --stat -p" {
		t.Errorf("git diff ref = %q", out)
	}
}

func TestExtraTool_GitCommitAddError(t *testing.T) {
	patchToolHooks(t)
	runGitFn = func(_ context.Context, args ...string) (string, error) {
		if len(args) > 0 && args[0] == "add" {
			return "add fail", errors.New("add failed")
		}
		return "", nil
	}
	if _, err := extraTool(context.Background(), "sin_git_commit", map[string]any{"message": "m"}); err == nil || !strings.Contains(err.Error(), "add failed") {
		t.Errorf("expected add error, got %v", err)
	}
}

func TestRunGit_TruncationAndError(t *testing.T) {
	dir := t.TempDir()
	fakeGit := filepath.Join(dir, "git")
	script := `#!/bin/sh
if [ "$1" = "error" ]; then echo "error"; exit 1; fi
yes | head -c 33000
`
	os.WriteFile(fakeGit, []byte(script), 0o755)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))

	out, err := runGit(context.Background(), "big")
	if err != nil {
		t.Fatalf("runGit big: %v", err)
	}
	if !strings.HasSuffix(out, "\n[... truncated]") {
		t.Errorf("expected truncation suffix, got %q", out)
	}

	out, err = runGit(context.Background(), "error")
	if err != nil {
		t.Fatalf("runGit error: %v", err)
	}
	if !strings.Contains(out, "git error") {
		t.Errorf("expected git error, got %q", out)
	}
}

func TestToolHTTPGet_Branches(t *testing.T) {
	patchToolHooks(t)
	ctx := context.Background()

	toolHTTPNewRequestFn = func(context.Context, string, string, io.Reader) (*http.Request, error) {
		return nil, errors.New("request fail")
	}
	if _, err := toolHTTPGet(ctx, "http://x"); err == nil || !strings.Contains(err.Error(), "request fail") {
		t.Errorf("expected request error, got %v", err)
	}

	toolHTTPNewRequestFn = http.NewRequestWithContext
	toolHTTPClientDoFn = func(*http.Request) (*http.Response, error) {
		return nil, errors.New("client fail")
	}
	if _, err := toolHTTPGet(ctx, "http://x"); err == nil || !strings.Contains(err.Error(), "client fail") {
		t.Errorf("expected client error, got %v", err)
	}

	toolHTTPClientDoFn = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(&errReader{err: errors.New("body fail")}),
		}, nil
	}
	if _, err := toolHTTPGet(ctx, "http://x"); err == nil || !strings.Contains(err.Error(), "body fail") {
		t.Errorf("expected body error, got %v", err)
	}

	toolHTTPClientDoFn = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("hello")),
		}, nil
	}
	out, err := toolHTTPGet(ctx, "http://x")
	if err != nil {
		t.Fatalf("toolHTTPGet success: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("output = %q", out)
	}
}

func TestToolHTTPClientDoFn_Default(t *testing.T) {
	patchToolHooks(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()
	req, err := http.NewRequest("GET", srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := toolHTTPClientDoFn(req)
	if err != nil {
		t.Fatalf("default client do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q", body)
	}
}

func TestToolTest_Branches(t *testing.T) {
	patchToolHooks(t)
	ctx := context.Background()
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(oldwd) })

	if _, err := toolTest(ctx, ""); err == nil || !strings.Contains(err.Error(), "no recognized test setup") {
		t.Errorf("expected default error, got %v", err)
	}

	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n"), 0o644)
	var gotCmd *exec.Cmd
	toolTestRunFn = func(cmd *exec.Cmd) ([]byte, error) {
		gotCmd = cmd
		return []byte("go ok"), nil
	}
	out, err := toolTest(ctx, "")
	if err != nil || !strings.Contains(out, "TEST PASS") {
		t.Errorf("go test = %q, %v", out, err)
	}
	if gotCmd == nil || !strings.Contains(strings.Join(gotCmd.Args, " "), "go test ./... -count=1") {
		t.Errorf("go cmd = %v", gotCmd)
	}

	out, err = toolTest(ctx, "./pkg")
	if err != nil || !strings.Contains(out, "TEST PASS") {
		t.Errorf("go test target = %q, %v", out, err)
	}
	if gotCmd == nil || !strings.Contains(strings.Join(gotCmd.Args, " "), "go test ./pkg -count=1") {
		t.Errorf("go target cmd = %v", gotCmd)
	}

	os.Remove(filepath.Join(dir, "go.mod"))
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644)
	toolTestRunFn = func(cmd *exec.Cmd) ([]byte, error) {
		gotCmd = cmd
		return []byte("npm ok"), nil
	}
	out, err = toolTest(ctx, "")
	if err != nil || !strings.Contains(out, "TEST PASS") {
		t.Errorf("npm test = %q, %v", out, err)
	}
	if gotCmd == nil || gotCmd.Path != "/bin/sh" {
		t.Errorf("npm cmd path = %q", gotCmd.Path)
	}

	os.Remove(filepath.Join(dir, "package.json"))
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o644)
	toolTestRunFn = func(cmd *exec.Cmd) ([]byte, error) {
		gotCmd = cmd
		return []byte("pytest ok"), nil
	}
	out, err = toolTest(ctx, "tests")
	if err != nil || !strings.Contains(out, "TEST PASS") {
		t.Errorf("pytest target = %q, %v", out, err)
	}
	if gotCmd == nil || !strings.Contains(strings.Join(gotCmd.Args, " "), "python3 -m pytest -q tests") {
		t.Errorf("pytest cmd = %v", gotCmd)
	}

	toolTestRunFn = func(cmd *exec.Cmd) ([]byte, error) {
		return []byte("fail"), errors.New("fail")
	}
	out, err = toolTest(ctx, "")
	if err != nil || !strings.Contains(out, "TEST FAIL") {
		t.Errorf("fail test = %q, %v", out, err)
	}

	toolTestRunFn = func(cmd *exec.Cmd) ([]byte, error) {
		return []byte(strings.Repeat("x", maxToolOutput+10)), nil
	}
	out, err = toolTest(ctx, "")
	if err != nil || !strings.HasSuffix(out, "\n[... truncated]") {
		t.Errorf("truncated test = %q", out)
	}
}

func TestToolTestRunFn_Default(t *testing.T) {
	patchToolHooks(t)
	out, err := toolTestRunFn(exec.Command("echo", "hello"))
	if err != nil {
		t.Fatalf("default run: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("out = %q", out)
	}
}
