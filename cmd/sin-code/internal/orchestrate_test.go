// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the orchestrate subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunOrchestrate_AddTask(t *testing.T) {
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

func TestRunOrchestrate_AddTaskMissingTitle(t *testing.T) {
	err := runOrchestrate("add", "", "", "", "text")
	if err == nil {
		t.Error("expected error when title is missing for add action")
	}
}

func TestRunOrchestrate_CompleteTask(t *testing.T) {
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

func TestRunOrchestrate_CompleteTaskNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := runOrchestrate("complete", "", "", "999", "text")
	if err == nil {
		t.Error("expected error when completing non-existent task")
	}
}

func TestRunOrchestrate_RemoveTask(t *testing.T) {
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

func TestRunOrchestrate_RemoveTaskNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := runOrchestrate("remove", "", "", "999", "text")
	if err == nil {
		t.Error("expected error when removing non-existent task")
	}
}

func TestRunOrchestrate_StatusTask(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Status task", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	if err := runOrchestrate("status", "", "", "1", "json"); err != nil {
		t.Fatalf("runOrchestrate status failed: %v", err)
	}
}

func TestRunOrchestrate_StatusTaskNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	err := runOrchestrate("status", "", "", "999", "text")
	if err == nil {
		t.Error("expected error when getting status of non-existent task")
	}
}

func TestRunOrchestrate_List(t *testing.T) {
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

func TestRunOrchestrate_ListJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	if err := runOrchestrate("add", "Task JSON", "", "", "text"); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
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

func TestRunOrchestrate_UnknownAction(t *testing.T) {
	err := runOrchestrate("unknown", "", "", "", "text")
	if err == nil {
		t.Error("expected error for unknown action")
	}
}

func TestParseID(t *testing.T) {
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

func TestSplitTags(t *testing.T) {
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

func TestLoadState_NewFile(t *testing.T) {
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

func TestLoadState_CorruptJSON(t *testing.T) {
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

func TestSaveState(t *testing.T) {
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
