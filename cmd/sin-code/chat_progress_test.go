// SPDX-License-Identifier: MIT
// Purpose: headless-mode progress output tests: ensure NDJSON progress
// events land on stderr while the stable JSON contract stays on stdout.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
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

// headlessChatHarness stubs the package-level chat hook variables so a
// headless run can complete without an LLM or external services.
func headlessChatHarness(t *testing.T) (
	restore func(),
	stdout *bytes.Buffer,
	stderr *bytes.Buffer,
) {
	t.Helper()

	t.Setenv("SIN_LLM_API_KEY", "test-key")
	t.Setenv("NVIDIA_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	origLoadAgent := chatLoadAgentFn
	origNewLLMClient := chatNewLLMClientFn
	origNewProviderCompletion := chatNewProviderCompletionFn
	origNewProviderCompletionFull := chatNewProviderCompletionFullFn
	origRulesForAgent := chatRulesForAgentFn
	origNewHooks := chatNewHooksFn
	origLoadMCPConfigs := chatLoadMCPConfigsFn
	origNewMCPManager := chatNewMCPManagerFn
	origMCPConnectAll := chatMCPConnectAllFn
	origOpenSession := chatOpenSessionFn
	origNewGate := chatNewGateFn
	origPrintResult := chatPrintResultFn
	origStdout := chatStdout
	origStderr := chatStderr

	dbPath := filepath.Join(t.TempDir(), "sessions.db")

	chatLoadAgentFn = func(name string) (orchestrator.AgentConfig, string, error) {
		return orchestrator.AgentConfig{}, "", nil
	}
	chatNewLLMClientFn = func(baseURL, apiKey string) *llm.Client {
		return &llm.Client{}
	}
	stubCompletion := func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return &agentloop.Completion{
			Text: "done",
			Raw:  session.Message{Role: "assistant", Content: "done"},
		}, nil
	}
	chatNewProviderCompletionFn = func(c *llm.Client, model string, maxTokens int, temperature float64) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return stubCompletion
	}
	chatNewProviderCompletionFullFn = func(c *llm.Client, model string, maxTokens int, temperature float64, cache *llm.PromptCache, thinking *agentloop.ThinkingConfig) func(context.Context, []session.Message, []agentloop.ToolSpec) (*agentloop.Completion, error) {
		return stubCompletion
	}
	chatRulesForAgentFn = func(cfg orchestrator.AgentConfig) []permission.Rule {
		return nil
	}
	chatNewHooksFn = func(loadErrs []hooks.Hook) *hooks.Engine {
		return hooks.New(nil)
	}
	chatLoadMCPConfigsFn = func(workspace string) []mcpclient.ServerConfig {
		return nil
	}
	chatNewMCPManagerFn = func(configs []mcpclient.ServerConfig) *mcpclient.Manager {
		return mcpclient.NewManager(nil)
	}
	chatMCPConnectAllFn = func(mgr *mcpclient.Manager, ctx context.Context) error {
		return nil
	}
	chatOpenSessionFn = func(path string) (*session.Store, error) {
		return session.Open(dbPath)
	}
	chatNewGateFn = func(mode string, poc, oracle verify.Runner) *verify.Gate {
		return verify.NewGate("off", nil, nil)
	}

	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	chatStdout = stdout
	chatStderr = stderr

	restore = func() {
		chatLoadAgentFn = origLoadAgent
		chatNewLLMClientFn = origNewLLMClient
		chatNewProviderCompletionFn = origNewProviderCompletion
		chatNewProviderCompletionFullFn = origNewProviderCompletionFull
		chatRulesForAgentFn = origRulesForAgent
		chatNewHooksFn = origNewHooks
		chatLoadMCPConfigsFn = origLoadMCPConfigs
		chatNewMCPManagerFn = origNewMCPManager
		chatMCPConnectAllFn = origMCPConnectAll
		chatOpenSessionFn = origOpenSession
		chatNewGateFn = origNewGate
		chatPrintResultFn = origPrintResult
		chatStdout = origStdout
		chatStderr = origStderr
	}
	return restore, stdout, stderr
}

func TestChat_HeadlessProgressJSON_EmitsNDJSONToStderr(t *testing.T) {
	restore, stdout, stderr := headlessChatHarness(t)
	defer restore()

	// Verify mode is off so the stubbed completion passes immediately.
	opts := &chatOptions{
		prompt:       "say hello",
		jsonOut:      true,
		progress:     "json",
		progressDest: "stderr",
		verifyMode:   "off",
	}

	if err := runChat(context.Background(), opts); err != nil {
		t.Fatalf("runChat failed: %v", err)
	}

	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	// Stdout must contain the stable JSON contract (pretty-printed by
	// printResult, so we parse the whole output as a single JSON document).
	var res agentloop.Result
	if err := json.Unmarshal([]byte(stdoutStr), &res); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdoutStr)
	}
	if !res.Verified {
		t.Errorf("expected verified=true, got %v", res.Verified)
	}
	if res.SessionID == "" {
		t.Errorf("expected non-empty session_id")
	}

	// Stderr must contain at least one NDJSON progress event.
	if stderrStr == "" {
		t.Fatalf("expected progress lines on stderr, got nothing")
	}
	stderrLines := strings.Split(stderrStr, "\n")
	foundProgress := false
	for _, line := range stderrLines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev agentloop.ProgressEvent
		if err := json.Unmarshal([]byte(line), &ev); err == nil && ev.Event != "" {
			foundProgress = true
			break
		}
	}
	if !foundProgress {
		t.Errorf("expected at least one valid progress event on stderr, got %q", stderrStr)
	}
}

func TestChat_HeadlessProgressOff_NoProgressOutput(t *testing.T) {
	restore, stdout, stderr := headlessChatHarness(t)
	defer restore()

	opts := &chatOptions{
		prompt:       "say hello",
		jsonOut:      true,
		progress:     "off",
		progressDest: "stderr",
		verifyMode:   "off",
	}

	if err := runChat(context.Background(), opts); err != nil {
		t.Fatalf("runChat failed: %v", err)
	}

	// Stdout must still be the stable JSON contract.
	stdoutStr := strings.TrimSpace(stdout.String())
	var res agentloop.Result
	if err := json.Unmarshal([]byte(stdoutStr), &res); err != nil {
		t.Fatalf("stdout not valid JSON: %v\n%s", err, stdoutStr)
	}

	// Stderr must contain no NDJSON progress lines (warnings may exist).
	stderrStr := strings.TrimSpace(stderr.String())
	for _, line := range strings.Split(stderrStr, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev agentloop.ProgressEvent
		if err := json.Unmarshal([]byte(line), &ev); err == nil && ev.Event != "" {
			t.Errorf("progress off should not emit progress events, got %q", line)
		}
	}
}

func TestChat_HeadlessProgressStdout_PreservesStderrJSON(t *testing.T) {
	restore, stdout, stderr := headlessChatHarness(t)
	defer restore()

	opts := &chatOptions{
		prompt:       "say hello",
		jsonOut:      true,
		progress:     "json",
		progressDest: "stdout",
		verifyMode:   "off",
	}

	if err := runChat(context.Background(), opts); err != nil {
		t.Fatalf("runChat failed: %v", err)
	}

	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	// Stderr must not contain progress NDJSON.
	for _, line := range strings.Split(stderrStr, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev agentloop.ProgressEvent
		if err := json.Unmarshal([]byte(line), &ev); err == nil && ev.Event != "" {
			t.Errorf("progress-dest=stdout should not write progress to stderr, got %q", line)
		}
	}

	// Stdout must contain the stable JSON result AND at least one progress
	// line somewhere. The JSON contract is pretty-printed, so we parse the
	// whole output as one Result and then inspect individual lines.
	lines := strings.Split(stdoutStr, "\n")
	var res agentloop.Result
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "{" || strings.HasPrefix(trimmed, "{") {
			candidate := strings.Join(lines[i:], "\n")
			if err := json.Unmarshal([]byte(candidate), &res); err == nil && res.SessionID != "" {
				break
			}
		}
	}
	if res.SessionID == "" {
		t.Fatalf("could not find stable JSON result in stdout: %q", stdoutStr)
	}
	foundProgress := false
	for _, line := range lines {
		var ev agentloop.ProgressEvent
		if err := json.Unmarshal([]byte(line), &ev); err == nil && ev.Event != "" {
			foundProgress = true
			break
		}
	}
	if !foundProgress {
		t.Errorf("expected progress events on stdout, got %q", stdoutStr)
	}
}

// keep os import alive if needed by other helpers in this file.
var _ = os.Getwd
