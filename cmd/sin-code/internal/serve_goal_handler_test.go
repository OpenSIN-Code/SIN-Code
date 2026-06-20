// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the sin_goal_* MCP tool handlers.
package internal

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
)

func setGoalQueuePath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	old := goalQueuePath
	goalQueuePath = func() string { return filepath.Join(dir, "goals.db") }
	t.Cleanup(func() { goalQueuePath = old })
}

func TestHandleGoalAdd_ReturnsGoalID(t *testing.T) {
	setGoalQueuePath(t)
	ctx := context.Background()

	out, err := handleGoalAdd(ctx, map[string]any{
		"prompt": "write a hello world in Go",
	})
	if err != nil {
		t.Fatalf("handleGoalAdd: %v", err)
	}

	var res map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	id, ok := res["goal_id"].(float64)
	if !ok || id < 1 {
		t.Fatalf("expected goal_id >= 1, got %v", res["goal_id"])
	}
	if res["status"] != "pending" {
		t.Fatalf("expected status pending, got %v", res["status"])
	}
}

func TestHandleGoalList_ContainsAddedGoal(t *testing.T) {
	setGoalQueuePath(t)
	ctx := context.Background()

	_, err := handleGoalAdd(ctx, map[string]any{
		"prompt": "list test goal",
	})
	if err != nil {
		t.Fatalf("handleGoalAdd: %v", err)
	}

	out, err := handleGoalList(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("handleGoalList: %v", err)
	}

	var goals []map[string]any
	if err := json.Unmarshal([]byte(out), &goals); err != nil {
		t.Fatalf("invalid JSON array: %v", err)
	}
	if len(goals) == 0 {
		t.Fatal("expected at least 1 goal in list")
	}
	found := false
	for _, g := range goals {
		if strings.Contains(g["prompt"].(string), "list test goal") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("added goal not found in list")
	}
}

func TestHandleGoalStatus_ReturnsGoalAndChildren(t *testing.T) {
	setGoalQueuePath(t)
	ctx := context.Background()

	addOut, err := handleGoalAdd(ctx, map[string]any{
		"prompt": "status test goal",
	})
	if err != nil {
		t.Fatalf("handleGoalAdd: %v", err)
	}
	var addRes map[string]any
	json.Unmarshal([]byte(addOut), &addRes)
	goalID := addRes["goal_id"].(float64)
	goalIDStr := strconv.FormatInt(int64(goalID), 10)

	out, err := handleGoalStatus(ctx, map[string]any{
		"id": goalIDStr,
	})
	if err != nil {
		t.Fatalf("handleGoalStatus: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	g, ok := payload["goal"].(map[string]any)
	if !ok {
		t.Fatal("missing 'goal' in response")
	}
	if g["prompt"] != "status test goal" {
		t.Fatalf("expected prompt 'status test goal', got %v", g["prompt"])
	}
	children, ok := payload["children"].([]any)
	if !ok {
		t.Fatal("missing 'children' in response")
	}
	if len(children) != 0 {
		t.Fatalf("expected 0 children, got %d", len(children))
	}
}

func TestHandleGoalComplete_StatusBecomesVerified(t *testing.T) {
	setGoalQueuePath(t)
	ctx := context.Background()

	addOut, err := handleGoalAdd(ctx, map[string]any{
		"prompt": "complete test goal",
	})
	if err != nil {
		t.Fatalf("handleGoalAdd: %v", err)
	}
	var addRes map[string]any
	json.Unmarshal([]byte(addOut), &addRes)
	goalID := addRes["goal_id"].(float64)
	goalIDStr := strconv.FormatInt(int64(goalID), 10)

	completeOut, err := handleGoalComplete(ctx, map[string]any{
		"id":      goalIDStr,
		"session": "test-session-1",
	})
	if err != nil {
		t.Fatalf("handleGoalComplete: %v", err)
	}
	var compRes map[string]any
	json.Unmarshal([]byte(completeOut), &compRes)
	if compRes["ok"] != true {
		t.Fatalf("expected ok=true, got %v", compRes["ok"])
	}

	statusOut, err := handleGoalStatus(ctx, map[string]any{
		"id": goalIDStr,
	})
	if err != nil {
		t.Fatalf("handleGoalStatus: %v", err)
	}
	var payload map[string]any
	json.Unmarshal([]byte(statusOut), &payload)
	g := payload["goal"].(map[string]any)
	if g["status"] != "verified" {
		t.Fatalf("expected status 'verified', got %v", g["status"])
	}
}

func TestHandleGoalAdd_WithCriteria_AttachesContract(t *testing.T) {
	setGoalQueuePath(t)
	ctx := context.Background()

	addOut, err := handleGoalAdd(ctx, map[string]any{
		"prompt":   "contract test goal",
		"criteria": []any{"task done", "tests pass"},
	})
	if err != nil {
		t.Fatalf("handleGoalAdd: %v", err)
	}
	var addRes map[string]any
	json.Unmarshal([]byte(addOut), &addRes)
	goalID := addRes["goal_id"].(float64)
	goalIDStr := strconv.FormatInt(int64(goalID), 10)

	statusOut, err := handleGoalStatus(ctx, map[string]any{
		"id": goalIDStr,
	})
	if err != nil {
		t.Fatalf("handleGoalStatus: %v", err)
	}
	var payload map[string]any
	json.Unmarshal([]byte(statusOut), &payload)
	g := payload["goal"].(map[string]any)
	contractStr, ok := g["contract"].(string)
	if !ok || contractStr == "" {
		t.Fatal("expected non-empty contract on goal")
	}

	c, perr := goalcontract.Unmarshal(contractStr)
	if perr != nil {
		t.Fatalf("contract unmarshal: %v", perr)
	}
	if len(c.SemanticCriteria) != 2 {
		t.Fatalf("expected 2 semantic criteria, got %d", len(c.SemanticCriteria))
	}
	if c.SemanticCriteria[0] != "task done" {
		t.Fatalf("expected first criterion 'task done', got %q", c.SemanticCriteria[0])
	}
}

func TestHandleGoalAdd_RequiresPrompt(t *testing.T) {
	setGoalQueuePath(t)
	if _, err := handleGoalAdd(context.Background(), map[string]any{}); err == nil {
		t.Fatal("missing prompt must error")
	}
}

func TestHandleGoalStatus_ParsesHashPrefix(t *testing.T) {
	setGoalQueuePath(t)
	ctx := context.Background()

	addOut, _ := handleGoalAdd(ctx, map[string]any{"prompt": "hash prefix test"})
	var addRes map[string]any
	json.Unmarshal([]byte(addOut), &addRes)
	goalID := addRes["goal_id"].(float64)

	idStr := "#" + strconv.FormatInt(int64(goalID), 10)
	out, err := handleGoalStatus(ctx, map[string]any{"id": idStr})
	if err != nil {
		t.Fatalf("handleGoalStatus with # prefix: %v", err)
	}
	var payload map[string]any
	json.Unmarshal([]byte(out), &payload)
	g := payload["goal"].(map[string]any)
	if g["prompt"] != "hash prefix test" {
		t.Fatalf("expected prompt match, got %v", g["prompt"])
	}
}
