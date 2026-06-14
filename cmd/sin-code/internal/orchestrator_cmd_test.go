// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the v2 orchestrator command helpers. (st-cov1)
package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/orchestrator"
)

func TestOrchestratorJoinIDs(t *testing.T) {
	tests := []struct {
		ids  []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b", "c"}, "a,b,c"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := joinIDs(tt.ids)
			if got != tt.want {
				t.Errorf("joinIDs(%v) = %q, want %q", tt.ids, got, tt.want)
			}
		})
	}
}

// TestLoadAllAgents_NoPlugins verifies that loadAllAgents returns only user
// agents when --no-plugins is set. (st-cov1)
func TestOrchestratorLoadAllAgents_NoPlugins(t *testing.T) {
	oldNoPlugins := orch2NoPlugins
	oldAgentsDir := orch2AgentsDir
	defer func() {
		orch2NoPlugins = oldNoPlugins
		orch2AgentsDir = oldAgentsDir
	}()

	orch2NoPlugins = true
	orch2AgentsDir = t.TempDir()

	agents, err := loadAllAgents()
	if err != nil {
		t.Fatalf("loadAllAgents failed: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents with empty dir and no plugins, got %d", len(agents))
	}
}

// TestRunOrchestrator_PlanOnly verifies that runOrchestrator with --plan-only
// produces a plan without dispatching agents. (st-cov1)
func TestOrchestrator_PlanOnly(t *testing.T) {
	oldPrompt := orch2Prompt
	oldPlanOnly := orch2PlanOnly
	oldFormat := orch2Format
	oldMaxParallel := orch2MaxParallel
	oldTimeout := orch2Timeout
	oldNoPlugins := orch2NoPlugins
	oldAgentsDir := orch2AgentsDir
	defer func() {
		orch2Prompt = oldPrompt
		orch2PlanOnly = oldPlanOnly
		orch2Format = oldFormat
		orch2MaxParallel = oldMaxParallel
		orch2Timeout = oldTimeout
		orch2NoPlugins = oldNoPlugins
		orch2AgentsDir = oldAgentsDir
	}()

	orch2Prompt = "add a test function"
	orch2PlanOnly = true
	orch2Format = "json"
	orch2MaxParallel = 2
	orch2Timeout = 0
	orch2NoPlugins = true
	orch2AgentsDir = t.TempDir()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runOrchestrator()

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runOrchestrator plan-only failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var plan orchestrator.Plan
	if err := json.Unmarshal(buf.Bytes(), &plan); err != nil {
		t.Fatalf("expected valid JSON plan, got %q: %v", buf.String(), err)
	}
	if plan.ID == "" {
		t.Error("expected non-empty plan ID")
	}
	if len(plan.Tasks) == 0 {
		t.Error("expected at least one planned task")
	}
}

// TestRunOrchestrator_PlanOnlyText verifies the text output path of plan-only
// mode. (st-cov1)
func TestOrchestrator_PlanOnlyText(t *testing.T) {
	oldPrompt := orch2Prompt
	oldPlanOnly := orch2PlanOnly
	oldFormat := orch2Format
	oldNoPlugins := orch2NoPlugins
	oldAgentsDir := orch2AgentsDir
	defer func() {
		orch2Prompt = oldPrompt
		orch2PlanOnly = oldPlanOnly
		orch2Format = oldFormat
		orch2NoPlugins = oldNoPlugins
		orch2AgentsDir = oldAgentsDir
	}()

	orch2Prompt = "refactor the auth module"
	orch2PlanOnly = true
	orch2Format = "text"
	orch2NoPlugins = true
	orch2AgentsDir = t.TempDir()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runOrchestrator()

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runOrchestrator plan-only text failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte("Plan")) {
		t.Errorf("expected text output to contain 'Plan', got %q", out)
	}
}

func captureOrchestratorCmd(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.RunE(cmd, args)
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String(), err
}

func TestOrchestratorAgentsCmd(t *testing.T) {
	oldNoPlugins := orch2NoPlugins
	oldFormat := orch2Format
	oldAgentsDir := orch2AgentsDir
	defer func() {
		orch2NoPlugins = oldNoPlugins
		orch2Format = oldFormat
		orch2AgentsDir = oldAgentsDir
	}()

	orch2NoPlugins = true
	orch2Format = "text"
	orch2AgentsDir = t.TempDir()

	out, err := captureOrchestratorCmd(t, OrchestratorAgentsCmd, []string{})
	if err != nil {
		t.Fatalf("OrchestratorAgentsCmd failed: %v", err)
	}
	if !strings.Contains(out, "Loaded") {
		t.Errorf("expected agents output, got %q", out)
	}
}

func TestOrchestratorAgentsCmdJSON(t *testing.T) {
	oldNoPlugins := orch2NoPlugins
	oldFormat := orch2Format
	oldAgentsDir := orch2AgentsDir
	defer func() {
		orch2NoPlugins = oldNoPlugins
		orch2Format = oldFormat
		orch2AgentsDir = oldAgentsDir
	}()

	orch2NoPlugins = true
	orch2Format = "json"
	orch2AgentsDir = t.TempDir()

	out, err := captureOrchestratorCmd(t, OrchestratorAgentsCmd, []string{})
	if err != nil {
		t.Fatalf("OrchestratorAgentsCmd json failed: %v", err)
	}
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected json array, got %q: %v", out, err)
	}
}

func TestOrchestratorPlanCmd(t *testing.T) {
	oldNoPlugins := orch2NoPlugins
	oldFormat := orch2Format
	oldAgentsDir := orch2AgentsDir
	defer func() {
		orch2NoPlugins = oldNoPlugins
		orch2Format = oldFormat
		orch2AgentsDir = oldAgentsDir
	}()

	orch2NoPlugins = true
	orch2Format = "text"
	orch2AgentsDir = t.TempDir()

	out, err := captureOrchestratorCmd(t, OrchestratorPlanCmd, []string{"add a test"})
	if err != nil {
		t.Fatalf("OrchestratorPlanCmd failed: %v", err)
	}
	if !strings.Contains(out, "Plan") {
		t.Errorf("expected plan output, got %q", out)
	}
}

// resetOrch2 saves and restores the package-level orchestrator globals.
func resetOrch2(t *testing.T) func() {
	t.Helper()
	oldPrompt, oldFormat, oldAgentsDir := orch2Prompt, orch2Format, orch2AgentsDir
	oldTimeout, oldMaxParallel := orch2Timeout, orch2MaxParallel
	oldPlanOnly, oldShowScratch, oldNoPlugins := orch2PlanOnly, orch2ShowScratch, orch2NoPlugins
	return func() {
		orch2Prompt, orch2Format, orch2AgentsDir = oldPrompt, oldFormat, oldAgentsDir
		orch2Timeout, orch2MaxParallel = oldTimeout, oldMaxParallel
		orch2PlanOnly, orch2ShowScratch, orch2NoPlugins = oldPlanOnly, oldShowScratch, oldNoPlugins
	}
}

// clearLLMEnv blanks provider API keys so tests use MockAgent instead of
// calling real LLM endpoints.
func clearLLMEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SIN_LLM_API_KEY", "SIN_NIM_API_KEY", "OPENAI_API_KEY",
		"ANTHROPIC_API_KEY", "GROQ_API_KEY", "SIN_LLM_BASE_URL",
	} {
		t.Setenv(key, "")
	}
}

// writeUserAgent creates a minimal user agent in baseDir/custom/agent.toml.
func writeUserAgent(t *testing.T, baseDir string) {
	t.Helper()
	agentDir := filepath.Join(baseDir, "custom")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "agent.toml"), []byte(`
name = "custom"
type = "code"
model = "mock"
`), 0644); err != nil {
		t.Fatal(err)
	}
}

// writePlugin creates a minimal plugin with one agent under configDir.
func writePlugin(t *testing.T, configDir string) {
	t.Helper()
	pluginDir := filepath.Join(configDir, "sin-code", "plugins", "testplugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.toml"), []byte(`
name = "testplugin"
version = "1.0.0"

[[agents]]
name = "helper"
type = "code"
model = "mock"
`), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestOrchestrator_RunCmdRunE(t *testing.T) {
	defer resetOrch2(t)()
	orch2NoPlugins = true
	orch2PlanOnly = true
	orch2Format = "text"
	orch2AgentsDir = t.TempDir()

	out, err := captureOrchestratorCmd(t, OrchestratorRunCmd, []string{"test prompt"})
	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	if !strings.Contains(out, "Plan") {
		t.Errorf("expected plan output, got %q", out)
	}
}

func TestOrchestrator_AgentsCmdLoadError(t *testing.T) {
	defer resetOrch2(t)()
	orch2NoPlugins = true
	agentsDir := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(agentsDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	orch2AgentsDir = agentsDir

	_, err := captureOrchestratorCmd(t, OrchestratorAgentsCmd, []string{})
	if err == nil {
		t.Error("expected error from loadAllAgents")
	}
}

func TestOrchestrator_AgentsCmdWithPlugin(t *testing.T) {
	defer resetOrch2(t)()
	tmp := t.TempDir()
	t.Setenv("SIN_CODE_CONFIG_DIR", tmp)
	writePlugin(t, tmp)
	orch2NoPlugins = false
	orch2Format = "text"
	orch2AgentsDir = t.TempDir()

	out, err := captureOrchestratorCmd(t, OrchestratorAgentsCmd, []string{})
	if err != nil {
		t.Fatalf("agents with plugin failed: %v", err)
	}
	if !strings.Contains(out, "[plugin testplugin]") {
		t.Errorf("expected plugin prefix, got %q", out)
	}
}

func TestOrchestrator_AgentsCmdWithPluginJSON(t *testing.T) {
	defer resetOrch2(t)()
	tmp := t.TempDir()
	t.Setenv("SIN_CODE_CONFIG_DIR", tmp)
	writePlugin(t, tmp)
	orch2NoPlugins = false
	orch2Format = "json"
	orch2AgentsDir = t.TempDir()

	out, err := captureOrchestratorCmd(t, OrchestratorAgentsCmd, []string{})
	if err != nil {
		t.Fatalf("agents json with plugin failed: %v", err)
	}
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected json array, got %q: %v", out, err)
	}
	found := false
	for _, e := range parsed {
		if src, ok := e["source"].(string); ok && src == "plugin" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected plugin source entry, got %q", out)
	}
}

func TestOrchestrator_PlanCmdLoadError(t *testing.T) {
	defer resetOrch2(t)()
	orch2NoPlugins = true
	agentsDir := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(agentsDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	orch2AgentsDir = agentsDir

	_, err := captureOrchestratorCmd(t, OrchestratorPlanCmd, []string{"test"})
	if err == nil {
		t.Error("expected error from loadAllAgents")
	}
}

func TestOrchestrator_PlanCmdJSON(t *testing.T) {
	defer resetOrch2(t)()
	orch2NoPlugins = true
	orch2Format = "json"
	orch2AgentsDir = t.TempDir()

	out, err := captureOrchestratorCmd(t, OrchestratorPlanCmd, []string{"test"})
	if err != nil {
		t.Fatalf("plan json failed: %v", err)
	}
	var plan orchestrator.Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("expected json plan, got %q: %v", out, err)
	}
	if plan.ID == "" {
		t.Error("expected plan ID")
	}
}

func TestOrchestrator_LoadAllAgentsError(t *testing.T) {
	defer resetOrch2(t)()
	orch2NoPlugins = true
	agentsDir := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(agentsDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	orch2AgentsDir = agentsDir

	_, err := loadAllAgents()
	if err == nil {
		t.Error("expected error from loadAllAgents")
	}
}

func TestOrchestrator_LoadAllAgentsUserNoPlugins(t *testing.T) {
	defer resetOrch2(t)()
	tmp := t.TempDir()
	t.Setenv("SIN_CODE_CONFIG_DIR", tmp)
	agentsDir := filepath.Join(tmp, "agents")
	writeUserAgent(t, agentsDir)
	orch2NoPlugins = false
	orch2AgentsDir = agentsDir

	agents, err := loadAllAgents()
	if err != nil {
		t.Fatalf("loadAllAgents failed: %v", err)
	}
	if len(agents) == 0 {
		t.Error("expected user agents")
	}
}

func TestOrchestrator_RunLoadAllAgentsError(t *testing.T) {
	defer resetOrch2(t)()
	orch2NoPlugins = true
	agentsDir := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(agentsDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	orch2AgentsDir = agentsDir

	err := runOrchestrator()
	if err == nil {
		t.Error("expected error from runOrchestrator")
	}
}

func TestOrchestrator_RunSuccessText(t *testing.T) {
	defer resetOrch2(t)()
	clearLLMEnv(t)
	orch2Prompt = "add a test"
	orch2PlanOnly = false
	orch2Format = "text"
	orch2ShowScratch = true
	orch2NoPlugins = true
	orch2AgentsDir = t.TempDir()
	orch2MaxParallel = 2
	orch2Timeout = 0

	out, err := captureOrchestratorCmd(t, OrchestratorRunCmd, []string{orch2Prompt})
	if err != nil {
		t.Fatalf("run text failed: %v", err)
	}
	if !strings.Contains(out, "Total:") {
		t.Errorf("expected summary output, got %q", out)
	}
	if !strings.Contains(out, "--- Scratchpad ---") {
		t.Errorf("expected scratchpad output, got %q", out)
	}
}

func TestOrchestrator_RunSuccessJSON(t *testing.T) {
	defer resetOrch2(t)()
	clearLLMEnv(t)
	orch2Prompt = "add a test"
	orch2PlanOnly = false
	orch2Format = "json"
	orch2ShowScratch = true
	orch2NoPlugins = true
	orch2AgentsDir = t.TempDir()
	orch2MaxParallel = 2
	orch2Timeout = 0

	out, err := captureOrchestratorCmd(t, OrchestratorRunCmd, []string{orch2Prompt})
	if err != nil {
		t.Fatalf("run json failed: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("expected json output, got %q: %v", out, err)
	}
	if _, ok := parsed["plan"]; !ok {
		t.Error("expected plan key")
	}
	if _, ok := parsed["scratchpad"]; !ok {
		t.Error("expected scratchpad key")
	}
}

func TestOrchestrator_RunError(t *testing.T) {
	defer resetOrch2(t)()
	clearLLMEnv(t)
	orch2Prompt = "test prompt"
	orch2PlanOnly = false
	orch2Format = "text"
	orch2ShowScratch = false
	orch2NoPlugins = true
	orch2AgentsDir = t.TempDir()
	orch2MaxParallel = 2
	orch2Timeout = 1

	_, err := captureOrchestratorCmd(t, OrchestratorRunCmd, []string{orch2Prompt})
	if err == nil {
		t.Error("expected runOrchestrator error")
	}
}
