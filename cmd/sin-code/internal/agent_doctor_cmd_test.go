// SPDX-License-Identifier: MIT
// Purpose: Unit tests for agent-show and agent-doctor commands. (st-cov1)
package internal

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/orchestrator"
)

func captureAgentCmd(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.RunE(cmd, args)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

func withIsolatedAgentConfig(t *testing.T) {
	t.Helper()
	old := os.Getenv("SIN_CODE_CONFIG_DIR")
	os.Setenv("SIN_CODE_CONFIG_DIR", t.TempDir())
	t.Cleanup(func() { os.Setenv("SIN_CODE_CONFIG_DIR", old) })
}

func TestAgentShow(t *testing.T) {
	withIsolatedAgentConfig(t)
	oldFormat := orch2Format
	orch2Format = "text"
	defer func() { orch2Format = oldFormat }()

	out, err := captureAgentCmd(t, OrchestratorAgentShowCmd, []string{"coder"})
	if err != nil {
		t.Fatalf("agent-show failed: %v", err)
	}
	if !strings.Contains(out, "Agent coder") {
		t.Errorf("expected agent show output, got %q", out)
	}
}

func TestAgentShowJSON(t *testing.T) {
	withIsolatedAgentConfig(t)
	oldFormat := orch2Format
	orch2Format = "json"
	defer func() { orch2Format = oldFormat }()

	out, err := captureAgentCmd(t, OrchestratorAgentShowCmd, []string{"coder"})
	if err != nil {
		t.Fatalf("agent-show json failed: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("agent-show json not valid: %v\n%q", err, out)
	}
}

func TestAgentDoctorOffline(t *testing.T) {
	withIsolatedAgentConfig(t)
	oldOffline := agDoctorOffline
	oldFormat := orch2Format
	agDoctorOffline = true
	orch2Format = "text"
	defer func() {
		agDoctorOffline = oldOffline
		orch2Format = oldFormat
	}()

	out, err := captureAgentCmd(t, OrchestratorAgentDoctorCmd, []string{})
	if err == nil {
		t.Fatal("agent-doctor expected error for default agents with missing keys")
	}
	if !strings.Contains(out, "Doctor") {
		t.Errorf("expected doctor output, got %q", out)
	}
}

func TestAgentDoctorFiltered(t *testing.T) {
	withIsolatedAgentConfig(t)
	oldOffline := agDoctorOffline
	oldFormat := orch2Format
	agDoctorOffline = true
	orch2Format = "text"
	defer func() {
		agDoctorOffline = oldOffline
		orch2Format = oldFormat
	}()

	out, err := captureAgentCmd(t, OrchestratorAgentDoctorCmd, []string{"coder"})
	if err == nil {
		t.Fatal("agent-doctor expected error for coder with missing keys")
	}
	if !strings.Contains(out, "coder") {
		t.Errorf("expected filtered agent output, got %q", out)
	}
}

func TestAgentDoctorWithMockServer(t *testing.T) {
	oldKey := os.Getenv("OPENAI_API_KEY")
	os.Setenv("OPENAI_API_KEY", "test-key")
	defer func() { os.Setenv("OPENAI_API_KEY", oldKey) }()

	oldFormat := orch2Format
	orch2Format = "json"
	defer func() { orch2Format = oldFormat }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"id": "test-model"}},
		})
	}))
	defer srv.Close()

	// Create a custom agent with baseURL pointing to mock server
	cfg := orchestrator.AgentConfig{
		Name:     "mock-agent",
		Provider: "openai",
		BaseURL:  srv.URL,
		Model:    "test-model",
	}
	rep := runDoctor([]orchestrator.AgentConfig{cfg}, false)
	if len(rep) != 1 {
		t.Fatalf("expected 1 report, got %d", len(rep))
	}
	if !rep[0].OK {
		t.Errorf("expected OK report, got issues %v", rep[0].Issues)
	}
}

func TestAgentDoctorUnknownProvider(t *testing.T) {
	cfg := orchestrator.AgentConfig{Name: "bad", Provider: "unknown-provider"}
	rep := runDoctor([]orchestrator.AgentConfig{cfg}, true)
	if len(rep) != 1 || rep[0].OK {
		t.Fatalf("expected failing report for unknown provider, got %+v", rep[0])
	}
}

func TestAgentDoctorMissingBaseURL(t *testing.T) {
	cfg := orchestrator.AgentConfig{Name: "nourl", Provider: "nim"}
	rep := runDoctor([]orchestrator.AgentConfig{cfg}, true)
	if len(rep) != 1 || rep[0].OK {
		t.Fatalf("expected failing report for missing base URL, got %+v", rep[0])
	}
}

func TestFetchModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"id": "model-a"}, {"id": "model-b"}},
		})
	}))
	defer srv.Close()

	models, err := fetchModels(srv.URL, "")
	if err != nil {
		t.Fatalf("fetchModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %v", models)
	}
}
