// SPDX-License-Identifier: MIT
// Purpose: MCP tool handlers for the autonomous goal queue (autonomy.Queue).
// Exposes sin_goal_add / sin_goal_list / sin_goal_status / sin_goal_complete
// so any MCP client can enqueue and manage goals that the sin-code daemon
// will execute with full Verify-Gate + Stop-Gate.
// Docs: serve_goal_handler.doc.md
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
)

var goalQueuePath = autonomy.DefaultPath

func openGoalQueue() (*autonomy.Queue, error) {
	return autonomy.Open(goalQueuePath())
}

func parseGoalIDMCP(s string) (int64, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid goal id %q: %w", s, err)
	}
	return id, nil
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func handleGoalAdd(ctx context.Context, args map[string]any) (string, error) {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	workspace := stringArg(args, "workspace", ".")
	if abs, err := filepath.Abs(workspace); err == nil {
		workspace = abs
	}
	priority := intArg(args, "priority", 0)
	retries := intArg(args, "retries", 3)
	criteria := toStringSlice(args["criteria"])

	q, err := openGoalQueue()
	if err != nil {
		return "", err
	}
	defer q.Close()

	var id int64
	if len(criteria) > 0 {
		c := &goalcontract.GoalContract{SemanticCriteria: criteria}
		contractJSON, mErr := c.Marshal()
		if mErr != nil {
			return "", fmt.Errorf("goal contract marshal: %w", mErr)
		}
		id, err = q.AddWithContract(ctx, prompt, workspace, priority, retries, contractJSON)
	} else {
		id, err = q.Add(ctx, prompt, workspace, priority, retries)
	}
	if err != nil {
		return "", err
	}

	out, _ := json.Marshal(map[string]any{
		"goal_id": id,
		"status":  "pending",
	})
	return string(out), nil
}

func handleGoalList(ctx context.Context, args map[string]any) (string, error) {
	status := stringArg(args, "status", "")
	format := stringArg(args, "format", "json")

	q, err := openGoalQueue()
	if err != nil {
		return "", err
	}
	defer q.Close()

	goals, err := q.List(ctx, autonomy.GoalStatus(status))
	if err != nil {
		return "", err
	}
	if goals == nil {
		goals = []autonomy.Goal{}
	}

	if format == "text" {
		var b strings.Builder
		if len(goals) == 0 {
			return "no goals", nil
		}
		fmt.Fprintf(&b, "%-5s %-10s %-4s %-8s %s\n", "ID", "STATUS", "TRY", "PRIO", "PROMPT")
		for _, g := range goals {
			fmt.Fprintf(&b, "%-5d %-10s %d/%-2d %-8d %.60s\n", g.ID, g.Status, g.Attempts, g.MaxRetries, g.Priority, g.Prompt)
		}
		return b.String(), nil
	}

	out, _ := json.MarshalIndent(goals, "", "  ")
	return string(out), nil
}

func handleGoalStatus(ctx context.Context, args map[string]any) (string, error) {
	idStr, _ := args["id"].(string)
	if idStr == "" {
		return "", fmt.Errorf("id is required")
	}
	id, err := parseGoalIDMCP(idStr)
	if err != nil {
		return "", err
	}
	format := stringArg(args, "format", "json")

	q, err := openGoalQueue()
	if err != nil {
		return "", err
	}
	defer q.Close()

	g, err := q.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if g == nil {
		return "", fmt.Errorf("goal %d not found", id)
	}
	children, err := q.Children(ctx, id)
	if err != nil {
		return "", err
	}
	if children == nil {
		children = []autonomy.Goal{}
	}

	if format == "text" {
		var b strings.Builder
		fmt.Fprintf(&b, "Goal %d [%s] attempts=%d/%d priority=%d\n", g.ID, g.Status, g.Attempts, g.MaxRetries, g.Priority)
		fmt.Fprintf(&b, "  prompt: %s\n", g.Prompt)
		if len(g.LastError) > 0 {
			fmt.Fprintf(&b, "  last_error: %s\n", g.LastError)
		}
		if len(children) == 0 {
			fmt.Fprintln(&b, "  (no subtasks)")
		} else {
			fmt.Fprintf(&b, "  subtasks (%d):\n", len(children))
			for _, c := range children {
				fmt.Fprintf(&b, "    %-5d [%-10s] %.60s\n", c.ID, c.Status, c.Prompt)
			}
		}
		return b.String(), nil
	}

	payload := map[string]any{
		"goal":     g,
		"children": children,
	}
	out, _ := json.MarshalIndent(payload, "", "  ")
	return string(out), nil
}

func handleGoalComplete(ctx context.Context, args map[string]any) (string, error) {
	idStr, _ := args["id"].(string)
	if idStr == "" {
		return "", fmt.Errorf("id is required")
	}
	id, err := parseGoalIDMCP(idStr)
	if err != nil {
		return "", err
	}
	session := stringArg(args, "session", "")

	q, err := openGoalQueue()
	if err != nil {
		return "", err
	}
	defer q.Close()

	if err := q.Complete(ctx, id, session); err != nil {
		return "", err
	}

	out, _ := json.Marshal(map[string]any{
		"ok":      true,
		"goal_id": id,
	})
	return string(out), nil
}
