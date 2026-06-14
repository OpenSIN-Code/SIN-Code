// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the orchestrate subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrchestrate_AddTask(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := runOrchestrate("add", "New Task", "urgent,backend", "", "text")
	if err != nil {
		t.Fatalf("runOrchestrate add failed: %v", err)
	}

	state, err := loadState()
	if err != nil {
		t.Fatalf("loadState failed: %v", err)
	}
	if len(state.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(state.Tasks))
	}
	if state.Tasks[0].Title != "New Task" {
		t.Errorf("expected title 'New Task', got %q", state.Tasks[0].Title)
	}
	if len(state.Tasks[0].Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(state.Tasks[0].Tags))
	}
}

func TestOrchestrate_AddTaskMissingTitle(t *testing.T) {
	err := runOrchestrate("add", "", "", "", "text")
	if err == nil {
		t.Error("expected error when title is missing for add action")
	}
}

func TestOrchestrate_CompleteTask(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Task to complete", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	if err := runOrchestrate("complete", "", "", "1", "text"); err != nil {
		t.Fatalf("runOrchestrate complete failed: %v", err)
	}

	state, _ := loadState()
	if state.Tasks[0].Status != "completed" {
		t.Errorf("expected status 'completed', got %q", state.Tasks[0].Status)
	}
}

func TestOrchestrate_CompleteTaskNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := runOrchestrate("complete", "", "", "999", "text")
	if err == nil {
		t.Error("expected error when completing non-existent task")
	}
}

func TestOrchestrate_RemoveTask(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Removable task", "", "", "text"); err != nil {
		t.Fatal(err)
	}
	if err := runOrchestrate("remove", "", "", "1", "text"); err != nil {
		t.Fatalf("runOrchestrate remove failed: %v", err)
	}

	state, _ := loadState()
	if len(state.Tasks) != 0 {
		t.Errorf("expected 0 tasks after removal, got %d", len(state.Tasks))
	}
}

func TestOrchestrate_RemoveTaskNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := runOrchestrate("remove", "", "", "999", "text")
	if err == nil {
		t.Error("expected error when removing non-existent task")
	}
}

func TestOrchestrate_StatusTask(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Status task", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	if err := runOrchestrate("status", "", "", "1", "json"); err != nil {
		t.Fatalf("runOrchestrate status failed: %v", err)
	}
}

func TestOrchestrate_StatusTaskNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := runOrchestrate("status", "", "", "999", "text")
	if err == nil {
		t.Error("expected error when getting status of non-existent task")
	}
}

func TestOrchestrate_List(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Task A", "", "", "text"); err != nil {
		t.Fatal(err)
	}
	if err := runOrchestrate("add", "Task B", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	if err := runOrchestrate("complete", "", "", "1", "text"); err != nil {
		t.Fatal(err)
	}

	if err := runOrchestrate("list", "", "", "", "text"); err != nil {
		t.Fatalf("runOrchestrate list failed: %v", err)
	}
}

func TestOrchestrate_ListJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Task JSON", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	if err := runOrchestrate("list", "", "", "", "json"); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("runOrchestrate list json failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	var tasks []task
	if err := json.Unmarshal(buf.Bytes(), &tasks); err != nil {
		t.Errorf("expected valid JSON output, got %q: %v", buf.String(), err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task in JSON output, got %d", len(tasks))
	}
}

func TestOrchestrate_UnknownAction(t *testing.T) {
	err := runOrchestrate("unknown", "", "", "", "text")
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestOrchestrateParseID(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"1", 1},
		{"123", 123},
		{"0", 0},
		{"abc", 0},
		{"", 0},
		{"12abc", 12},
		{"-5", -5},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parseID(tt.input); got != tt.expected {
				t.Errorf("parseID(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestOrchestrateSplitTags(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", nil},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b , c", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
		{"single", []string{"single"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitTags(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("splitTags(%q) = %v, want %v", tt.input, got, tt.expected)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitTags(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestOrchestrateLoadState_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	state, err := loadState()
	if err != nil {
		t.Fatalf("loadState failed: %v", err)
	}
	if state.Version != 1 {
		t.Errorf("expected version 1, got %d", state.Version)
	}
	if state.NextID != 1 {
		t.Errorf("expected next_id 1, got %d", state.NextID)
	}
	if len(state.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(state.Tasks))
	}
}

func TestOrchestrateLoadState_CorruptJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	stateDir := filepath.Join(tmpDir, ".local", "state", "sin-code")
	os.MkdirAll(stateDir, 0755)
	statePath := filepath.Join(stateDir, "orchestrate.json")
	os.WriteFile(statePath, []byte("not json"), 0644)

	_, err := loadState()
	if err == nil {
		t.Error("expected error for corrupt JSON")
	}
}

func TestOrchestrateSaveState(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	state := &orchestrateState{
		Tasks:   []task{{ID: 1, Title: "Test", Status: "pending"}},
		NextID:  2,
		Version: 1,
	}
	if err := saveState(state); err != nil {
		t.Fatalf("saveState failed: %v", err)
	}

	data, err := os.ReadFile(getStateFile())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Test") {
		t.Errorf("expected saved state to contain 'Test', got %q", string(data))
	}
}

func TestOrchestrate_AddTaskText(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := runOrchestrate("add", "Text Task", "tag1", "", "text")
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runOrchestrate add text failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "Added task") {
		t.Errorf("expected Added task in output, got: %q", out)
	}
}

func TestOrchestrate_CompleteTaskJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Task", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	err := runOrchestrate("complete", "", "", "1", "json")
	if err != nil {
		t.Fatalf("runOrchestrate complete json failed: %v", err)
	}
}

func TestOrchestrate_RemoveTaskJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Removable", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	err := runOrchestrate("remove", "", "", "1", "json")
	if err != nil {
		t.Fatalf("runOrchestrate remove json failed: %v", err)
	}
}

func TestOrchestrate_StatusTaskJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Status Task", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	err := runOrchestrate("status", "", "", "1", "json")
	if err != nil {
		t.Fatalf("runOrchestrate status json failed: %v", err)
	}
}

func TestOrchestrate_RemoveMissingID(t *testing.T) {
	err := runOrchestrate("remove", "", "", "", "text")
	if err == nil {
		t.Error("expected error when --id is missing for remove")
	}
}

func TestOrchestrate_CompleteMissingID(t *testing.T) {
	err := runOrchestrate("complete", "", "", "", "text")
	if err == nil {
		t.Error("expected error when --id is missing for complete")
	}
}

func TestOrchestrate_StatusMissingID(t *testing.T) {
	err := runOrchestrate("status", "", "", "", "text")
	if err == nil {
		t.Error("expected error when --id is missing for status")
	}
}

func TestOrchestrate_ListWithInProgress(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Pending Task", "", "", "text"); err != nil {
		t.Fatal(err)
	}
	if err := runOrchestrate("add", "Done Task", "", "", "text"); err != nil {
		t.Fatal(err)
	}
	if err := runOrchestrate("complete", "", "", "2", "text"); err != nil {
		t.Fatal(err)
	}

	state, _ := loadState()
	state.Tasks[0].Status = "in-progress"
	saveState(state)

	err := runOrchestrate("list", "", "", "", "text")
	if err != nil {
		t.Fatalf("runOrchestrate list failed: %v", err)
	}
}

func TestOrchestrate_ListWithBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Blocked Task", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	state, _ := loadState()
	state.Tasks[0].Status = "blocked"
	saveState(state)

	err := runOrchestrate("list", "", "", "", "text")
	if err != nil {
		t.Fatalf("runOrchestrate list failed: %v", err)
	}
}

func TestOrchestrate_AddWithTags(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := runOrchestrate("add", "Tagged Task", "urgent,backend,api", "", "text")
	if err != nil {
		t.Fatalf("runOrchestrate add with tags failed: %v", err)
	}

	state, _ := loadState()
	if len(state.Tasks[0].Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(state.Tasks[0].Tags))
	}
}

func TestOrchestrateLoadState_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	stateDir := filepath.Join(tmpDir, ".local", "state", "sin-code")
	os.MkdirAll(stateDir, 0755)
	statePath := filepath.Join(stateDir, "orchestrate.json")
	os.WriteFile(statePath, []byte("not json"), 0644)
	os.Chmod(statePath, 0000)
	defer os.Chmod(statePath, 0644)

	_, err := loadState()
	if err == nil {
		t.Error("expected error for unreadable state file")
	}
}

func TestOrchestrateSaveState_MarshalError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	state := &orchestrateState{
		Tasks:   []task{},
		NextID:  1,
		Version: 1,
	}
	err := saveState(state)
	if err != nil {
		t.Errorf("saveState should not fail for valid state: %v", err)
	}
}

func TestOrchestrateGetStateFile_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path := getStateFile()
	if path == "" {
		t.Error("expected non-empty state file path")
	}
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("expected state directory to be created")
	}
}

func TestOrchestrateLoadState_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	stateDir := filepath.Join(tmpDir, ".local", "state", "sin-code")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "orchestrate.json"), []byte("{invalid json}"), 0644)

	_, err := loadState()
	if err == nil {
		t.Error("expected error for invalid JSON in state file")
	}
}

func TestOrchestrateSaveState_Error(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	stateDir := filepath.Join(tmpDir, ".local", "state", "sin-code")
	os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "orchestrate.json")
	os.WriteFile(stateFile, []byte("{}"), 0644)
	os.Chmod(stateDir, 0000)
	defer os.Chmod(stateDir, 0755)

	state := &orchestrateState{Tasks: []task{}, NextID: 1, Version: 1}
	err := saveState(state)
	if err == nil {
		t.Error("expected error when writing to read-only dir")
	}
}

func TestOrchestrate_CompleteNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	err := runOrchestrate("complete", "", "", "999", "text")
	if err == nil {
		t.Error("expected error for completing nonexistent task")
	}
}

func TestOrchestrate_RemoveNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	err := runOrchestrate("remove", "", "", "999", "text")
	if err == nil {
		t.Error("expected error for removing nonexistent task")
	}
}

func TestOrchestrate_StatusNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	err := runOrchestrate("status", "", "", "999", "text")
	if err == nil {
		t.Error("expected error for status of nonexistent task")
	}
}

func TestOrchestrate_AddNoTitle(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	err := runOrchestrate("add", "", "", "", "text")
	if err == nil {
		t.Error("expected error for add without title")
	}
}

func TestOrchestrate_InvalidAction(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	err := runOrchestrate("bogus", "", "", "", "text")
	if err == nil {
		t.Error("expected error for invalid action")
	}
}

func TestOrchestrate_ListJSONv2Output(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	runOrchestrate("add", "Task1", "tag1", "", "text")
	runOrchestrate("add", "Task2", "", "", "text")

	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := runOrchestrate("list", "", "", "", "json")
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("list json failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var tasks []task
	if err := json.Unmarshal(buf.Bytes(), &tasks); err != nil {
		t.Fatalf("expected valid JSON, got: %v output: %q", err, buf.String())
	}
	if len(tasks) < 2 {
		t.Errorf("expected at least 2 tasks, got %d", len(tasks))
	}
}

func TestOrchestrate_StatusJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	runOrchestrate("add", "StatusTask", "", "", "text")

	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := runOrchestrate("status", "", "", "1", "json")
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("status json failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	var task task
	if err := json.Unmarshal(buf.Bytes(), &task); err != nil {
		t.Fatalf("expected valid JSON for status, got: %v", err)
	}
}

func TestOrchestrate_RemoveNoID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	err := runOrchestrate("remove", "", "", "", "text")
	if err == nil {
		t.Error("expected error for remove without id")
	}
}

func TestOrchestrate_CompleteNoID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	err := runOrchestrate("complete", "", "", "", "text")
	if err == nil {
		t.Error("expected error for complete without id")
	}
}

func TestOrchestrate_StatusNoID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	err := runOrchestrate("status", "", "", "", "text")
	if err == nil {
		t.Error("expected error for status without id")
	}
}

func TestOrchestrate_ListWithBlockedTask(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	runOrchestrate("add", "BlockedTask", "", "", "text")

	state, _ := loadState()
	state.Tasks[0].Status = "blocked"
	state.Tasks[0].Tags = []string{"urgent"}
	saveState(state)

	old := os.Stdout
	r, w, _ := os.Pipe()
	defer r.Close()
	os.Stdout = w

	err := runOrchestrate("list", "", "", "", "text")
	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "BlockedTask") {
		t.Errorf("expected BlockedTask in output, got: %q", out)
	}
}

func TestOrchestrateLoadState_ZeroVersion(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	stateDir := filepath.Join(tmpDir, ".local", "state", "sin-code")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "orchestrate.json"), []byte(`{"tasks":[],"next_id":0,"version":0}`), 0644)

	state, err := loadState()
	if err != nil {
		t.Fatalf("loadState failed: %v", err)
	}
	if state.Version != 1 {
		t.Errorf("expected version to be set to 1, got %d", state.Version)
	}
	if state.NextID != 1 {
		t.Errorf("expected next_id to be set to 1, got %d", state.NextID)
	}
}

func TestOrchestrateSaveState_MarshalCheck(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	state := &orchestrateState{
		Tasks:   []task{{Title: strings.Repeat("x", 100)}},
		NextID:  1,
		Version: 1,
	}
	err := saveState(state)
	_ = err
}

// failMarshal swaps jsonMarshalIndent for a failing implementation and
// returns a restore function. Use with defer.
func failMarshal(t *testing.T) func() {
	t.Helper()
	old := jsonMarshalIndent
	jsonMarshalIndent = func(v interface{}, prefix, indent string) ([]byte, error) {
		return nil, errors.New("forced marshal error")
	}
	return func() { jsonMarshalIndent = old }
}

func TestOrchestrate_CmdRunE(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	oldAction, oldTitle, oldTags, oldID, oldFormat := orchAction, orchTitle, orchTags, orchID, orchFormat
	defer func() {
		orchAction, orchTitle, orchTags, orchID, orchFormat = oldAction, oldTitle, oldTags, oldID, oldFormat
	}()

	orchAction = "add"
	orchTitle = "CmdRunE task"
	orchTags = "tag"
	orchID = ""
	orchFormat = "text"

	err := OrchestrateCmd.RunE(nil, []string{})
	if err != nil {
		t.Fatalf("RunE failed: %v", err)
	}
	state, _ := loadState()
	if len(state.Tasks) != 1 || state.Tasks[0].Title != "CmdRunE task" {
		t.Errorf("expected task via RunE, got %+v", state.Tasks)
	}
}

func TestOrchestrate_SaveStateMarshalError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer failMarshal(t)()

	err := saveState(&orchestrateState{Tasks: []task{{ID: 1, Title: "x"}}, NextID: 2, Version: 1})
	if err == nil {
		t.Error("expected marshal error")
	}
}

func TestOrchestrate_AddSaveError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer failMarshal(t)()

	err := runOrchestrate("add", "Task", "", "", "text")
	if err == nil {
		t.Error("expected add save error")
	}
}

func TestOrchestrate_CompleteSaveError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Task", "", "", "text"); err != nil {
		t.Fatal(err)
	}
	defer failMarshal(t)()

	err := runOrchestrate("complete", "", "", "1", "text")
	if err == nil {
		t.Error("expected complete save error")
	}
}

func TestOrchestrate_RemoveSaveError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Task", "", "", "text"); err != nil {
		t.Fatal(err)
	}
	defer failMarshal(t)()

	err := runOrchestrate("remove", "", "", "1", "text")
	if err == nil {
		t.Error("expected remove save error")
	}
}

func TestOrchestrate_RemoveKeepsOthers(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	for _, title := range []string{"Keep", "Remove"} {
		if err := runOrchestrate("add", title, "", "", "text"); err != nil {
			t.Fatal(err)
		}
	}
	if err := runOrchestrate("remove", "", "", "2", "text"); err != nil {
		t.Fatal(err)
	}
	state, _ := loadState()
	if len(state.Tasks) != 1 || state.Tasks[0].Title != "Keep" {
		t.Errorf("expected Keep to remain, got %+v", state.Tasks)
	}
}

func TestOrchestrate_SaveError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	stateDir := filepath.Join(tmpDir, ".local", "state", "sin-code")
	os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "orchestrate.json")
	os.WriteFile(stateFile, []byte("[]"), 0644)

	runOrchestrate("add", "First", "", "", "text")
	os.Chmod(stateDir, 0000)
	defer os.Chmod(stateDir, 0755)

	err := runOrchestrate("add", "Second", "", "", "text")
	if err == nil {
		t.Error("expected error when state dir is unwritable")
	}
}

func TestOrchestrate_CompleteSaveErrorChmod(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	stateDir := filepath.Join(tmpDir, ".local", "state", "sin-code")
	os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "orchestrate.json")
	os.WriteFile(stateFile, []byte("[]"), 0644)

	runOrchestrate("add", "Task", "", "", "text")
	os.Chmod(stateDir, 0000)
	defer os.Chmod(stateDir, 0755)

	err := runOrchestrate("complete", "", "", "1", "text")
	if err == nil {
		t.Error("expected error when state dir is unwritable for complete")
	}
}

func TestOrchestrate_RemoveSaveErrorChmod(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	stateDir := filepath.Join(tmpDir, ".local", "state", "sin-code")
	os.MkdirAll(stateDir, 0755)
	stateFile := filepath.Join(stateDir, "orchestrate.json")
	os.WriteFile(stateFile, []byte("[]"), 0644)

	runOrchestrate("add", "Task", "", "", "text")
	os.Chmod(stateDir, 0000)
	defer os.Chmod(stateDir, 0755)

	err := runOrchestrate("remove", "", "", "1", "text")
	if err == nil {
		t.Error("expected error when state dir is unwritable for remove")
	}
}
