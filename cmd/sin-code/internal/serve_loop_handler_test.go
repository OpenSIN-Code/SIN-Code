// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the sin_run_loop MCP handler.
package internal

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

func TestRunLoop_EmptyPromptErrors(t *testing.T) {
	_, err := handleRunLoop(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("empty prompt must error")
	}
	if !strings.Contains(err.Error(), "prompt is required") {
		t.Fatalf("expected 'prompt is required' error, got: %v", err)
	}
}

func TestRunLoop_BuildLoopOptions_Defaults(t *testing.T) {
	opts, err := buildLoopOptions(map[string]any{
		"prompt": "write hello world",
	})
	if err != nil {
		t.Fatalf("buildLoopOptions failed: %v", err)
	}
	if !opts.Headless {
		t.Error("expected Headless=true")
	}
	if opts.MaxTurns != 80 {
		t.Errorf("expected MaxTurns=80, got %d", opts.MaxTurns)
	}
	if !opts.SkipMCP {
		t.Error("expected SkipMCP=true")
	}
	if opts.VerifyMode != "off" {
		t.Errorf("expected VerifyMode=off (no verify_cmd, no criteria), got %q", opts.VerifyMode)
	}
	if opts.Style != "default" {
		t.Errorf("expected Style=default, got %q", opts.Style)
	}
	if opts.Contract != nil {
		t.Error("expected Contract=nil when no criteria")
	}
}

func TestRunLoop_BuildLoopOptions_VerifyCmdSetsPoc(t *testing.T) {
	opts, err := buildLoopOptions(map[string]any{
		"prompt":     "write hello world",
		"verify_cmd": "go build ./...",
	})
	if err != nil {
		t.Fatalf("buildLoopOptions failed: %v", err)
	}
	if opts.VerifyMode != "poc" {
		t.Errorf("expected VerifyMode=poc, got %q", opts.VerifyMode)
	}
	if opts.VerifyCmd != "go build ./..." {
		t.Errorf("expected VerifyCmd='go build ./...', got %q", opts.VerifyCmd)
	}
}

func TestRunLoop_BuildLoopOptions_CriteriaActivatesContract(t *testing.T) {
	opts, err := buildLoopOptions(map[string]any{
		"prompt":    "write hello world",
		"criteria":  []any{"task completed", "tests pass"},
		"max_turns": 40,
		"yolo":      true,
		"style":     "terse",
		"model":     "test-model",
		"agent":     "test-agent",
	})
	if err != nil {
		t.Fatalf("buildLoopOptions failed: %v", err)
	}
	if opts.Contract == nil {
		t.Fatal("expected Contract to be non-nil when criteria provided")
	}
	if opts.Contract.IsEmpty() {
		t.Fatal("expected Contract to be non-empty when criteria provided")
	}
	if len(opts.Contract.SemanticCriteria) != 2 {
		t.Fatalf("expected 2 semantic criteria, got %d", len(opts.Contract.SemanticCriteria))
	}
	if opts.Contract.SemanticCriteria[0] != "task completed" {
		t.Errorf("expected first criterion 'task completed', got %q", opts.Contract.SemanticCriteria[0])
	}
	if opts.VerifyMode != "poc" {
		t.Errorf("expected VerifyMode=poc (criteria present), got %q", opts.VerifyMode)
	}
	if opts.MaxTurns != 40 {
		t.Errorf("expected MaxTurns=40, got %d", opts.MaxTurns)
	}
	if !opts.Yolo {
		t.Error("expected Yolo=true")
	}
	if opts.Style != "terse" {
		t.Errorf("expected Style=terse, got %q", opts.Style)
	}
	if opts.Model != "test-model" {
		t.Errorf("expected Model=test-model, got %q", opts.Model)
	}
	if opts.AgentName != "test-agent" {
		t.Errorf("expected AgentName=test-agent, got %q", opts.AgentName)
	}
}

func TestRunLoop_FullHandlerReturnsJSON(t *testing.T) {
	origFactory := runLoopFactory
	origLessons := runLoopLessonsOpenFn
	origSession := runLoopSessionOpenFn
	defer func() {
		runLoopFactory = origFactory
		runLoopLessonsOpenFn = origLessons
		runLoopSessionOpenFn = origSession
	}()

	dir := t.TempDir()
	ls, err := lessons.Open(filepath.Join(dir, "lessons.db"))
	if err != nil {
		t.Fatalf("open test lessons store: %v", err)
	}
	defer ls.Close()

	runLoopLessonsOpenFn = func(path string) (*lessons.Store, error) {
		return ls, nil
	}
	sessStore, err := session.Open(filepath.Join(dir, "sessions.db"))
	if err != nil {
		t.Fatalf("open test session store: %v", err)
	}
	defer sessStore.Close()
	runLoopSessionOpenFn = func(dbPath string) (*session.Store, error) {
		return sessStore, nil
	}
	runLoopFactory = func(ctx context.Context, opts RunLoopOptions, ls2 *lessons.Store) (*agentloop.Loop, func() error, error) {
		loop := &agentloop.Loop{
			Workspace: opts.Workspace,
			MaxTurns:  opts.MaxTurns,
			SessionID: "test-session-id",
			Completion: func(ctx context.Context, hist []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
				return &agentloop.Completion{Text: "done"}, nil
			},
			RunOverride: func(ctx context.Context, sess *session.Session, prompt string) (*agentloop.Result, error) {
				return &agentloop.Result{
					SessionID: sess.ID,
					Summary:   "task completed successfully",
					Verified:  true,
					Turns:     3,
				}, nil
			},
		}
		return loop, func() error { return nil }, nil
	}

	out, err := handleRunLoop(context.Background(), map[string]any{
		"prompt":    "write hello world",
		"workspace": dir,
	})
	if err != nil {
		t.Fatalf("handleRunLoop failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\noutput: %s", err, out)
	}
	if result["session_id"] == nil || result["session_id"] == "" {
		t.Error("expected non-empty session_id in output")
	}
	if result["verified"] != true {
		t.Errorf("expected verified=true, got %v", result["verified"])
	}
	if result["turns"] == nil {
		t.Error("expected turns field in output")
	}
	if result["summary"] == nil || result["summary"] == "" {
		t.Error("expected non-empty summary in output")
	}
}
