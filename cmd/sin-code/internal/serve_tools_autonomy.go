// SPDX-License-Identifier: MIT
// Purpose: serve — tool definitions for autonomy, vision, and goal tools.
// sin-debt: shrink, upgrade: consolidate when serve handlers are refactored
package internal

// autonomyToolDefs returns the tool definitions for vision, agent-loop
// delegation, and autonomous goal tools: analyse_image, run_loop,
// goal_add, goal_list, goal_status, goal_complete.
func autonomyToolDefs() []toolDef {
	return []toolDef{
		{
			name:        "sin_analyse_image",
			description: "Analyze an image with a vision-capable LLM (no Tesseract). Returns a structured description including visible text, UI elements, and layout.",
			handler:     handleAnalyseImage,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Path to the image file"},
					"prompt": map[string]any{"type": "string", "description": "Custom prompt for the vision model (optional)"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_run_loop",
			description: "Run a prompt through the full SIN-Code agent loop (PLAN→ACT→VERIFY→DONE). Returns {session_id, summary, verified, turns}. Blocks until completion. Includes Verify-Gate, Stop-Gate, Lessons, Compaction, Loop-Detection. This is the synchronous delegation path — one call, one verified task.",
			handler:     handleRunLoop,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt":     map[string]any{"type": "string"},
					"workspace":  map[string]any{"type": "string", "default": "."},
					"model":      map[string]any{"type": "string"},
					"max_turns":  map[string]any{"type": "integer", "default": 80},
					"verify_cmd": map[string]any{"type": "string"},
					"yolo":       map[string]any{"type": "boolean", "default": false},
					"agent":      map[string]any{"type": "string"},
					"style":      map[string]any{"type": "string", "enum": []string{"default", "verbose", "normal", "terse", "ultra"}, "default": "default"},
					"criteria":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"prompt"},
			},
		},
		{
			name:        "sin_goal_add",
			description: "Enqueue a goal for autonomous execution by the sin-code daemon. Returns immediately with the goal ID. The daemon will pick it up, run the full agent loop (with Verify-Gate, Stop-Gate, Lessons), and mark it verified/failed. Use sin_goal_status to poll progress.",
			handler:     handleGoalAdd,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prompt":    map[string]any{"type": "string"},
					"workspace": map[string]any{"type": "string", "default": "."},
					"priority":  map[string]any{"type": "integer", "default": 0},
					"retries":   map[string]any{"type": "integer", "default": 3},
					"criteria":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"prompt"},
			},
		},
		{
			name:        "sin_goal_list",
			description: "List goals in the autonomous goal queue. Filter by status: pending, running, verified, failed, exhausted.",
			handler:     handleGoalList,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{"type": "string", "enum": []string{"pending", "running", "verified", "failed", "exhausted"}},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
			},
		},
		{
			name:        "sin_goal_status",
			description: "Show one goal's progress, attempts, and children (subtasks).",
			handler:     handleGoalStatus,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":     map[string]any{"type": "string"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"id"},
			},
		},
		{
			name:        "sin_goal_complete",
			description: "Mark a goal as verified/done. Maps to autonomy.Queue.Complete().",
			handler:     handleGoalComplete,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":      map[string]any{"type": "string"},
					"session": map[string]any{"type": "string"},
				},
				"required": []string{"id"},
			},
		},
	}
}
