// SPDX-License-Identifier: MIT
// Purpose: serve — tool definitions for todo tracker tools.
// sin-debt: shrink, upgrade: consolidate when serve handlers are refactored
package internal

// todoToolDefs returns the tool definitions for all todo_* tools:
// add, list, show, complete, claim, ready, blocked, search, prime,
// stats, dep_add, deps.
func todoToolDefs() []toolDef {
	return []toolDef{
		{
			name:        "sin_todo_add",
			description: "Add a todo (v2 bbolt store, hash ID, supports priority/type/tags/project/assignee)",
			handler:     handleTodoAdd,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"title"},
				"properties": map[string]any{
					"title":       map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"priority":    map[string]any{"type": "string", "enum": []string{"P0", "P1", "P2", "P3"}, "default": "P2"},
					"type":        map[string]any{"type": "string", "enum": []string{"task", "bug", "feature", "chore", "epic", "question"}, "default": "task"},
					"tags":        map[string]any{"type": "string", "description": "Comma-separated"},
					"project":     map[string]any{"type": "string"},
					"assignee":    map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_todo_list",
			description: "List todos with filters (status/priority/tag/project/limit)",
			handler:     handleTodoList,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status":   map[string]any{"type": "string"},
					"priority": map[string]any{"type": "string"},
					"tag":      map[string]any{"type": "string"},
					"project":  map[string]any{"type": "string"},
					"limit":    map[string]any{"type": "integer", "default": 50},
				},
			},
		},
		{
			name:        "sin_todo_show",
			description: "Show full details of a todo by ID (includes audit log + dependencies)",
			handler:     handleTodoShow,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_todo_complete",
			description: "Mark a todo as done (status=done, sets closed_at, fires hooks+notifications)",
			handler:     handleTodoComplete,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_todo_claim",
			description: "Atomically claim a todo (assigns to --as, sets status=in_progress)",
			handler:     handleTodoClaim,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
					"as": map[string]any{"type": "string", "description": "Actor (default git user.name)"},
				},
			},
		},
		{
			name:        "sin_todo_ready",
			description: "List unblocked open work (P0 first) — what should I work on next?",
			handler:     handleTodoReady,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_todo_blocked",
			description: "List blocked todos (have open dependencies)",
			handler:     handleTodoBlocked,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_todo_search",
			description: "Full-text search in todo titles + descriptions",
			handler:     handleTodoSearch,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_todo_prime",
			description: "Print ready/blocked/mine context for agent prompts",
			handler:     handleTodoPrime,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_todo_stats",
			description: "Counts by status/priority/type/assignee (JSON)",
			handler:     handleTodoStats,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_todo_dep_add",
			description: "Add a dependency between two todos (child depends on parent)",
			handler:     handleTodoDepAdd,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"child", "parent"},
				"properties": map[string]any{
					"child":  map[string]any{"type": "string"},
					"parent": map[string]any{"type": "string"},
					"rel":    map[string]any{"type": "string", "enum": []string{"blocks", "parent-child", "related", "discovered-from", "duplicates", "supersedes"}, "default": "blocks"},
				},
			},
		},
		{
			name:        "sin_todo_deps",
			description: "Show dependency tree of a todo",
			handler:     handleTodoDep,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"child"},
				"properties": map[string]any{
					"child": map[string]any{"type": "string"},
				},
			},
		},
	}
}
