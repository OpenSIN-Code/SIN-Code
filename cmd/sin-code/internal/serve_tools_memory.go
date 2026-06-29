// SPDX-License-Identifier: MIT
// Purpose: serve — tool definitions for memory and notifications tools.
// sin-debt: shrink, upgrade: consolidate when serve handlers are refactored
package internal

// memoryToolDefs returns the tool definitions for memory_* and
// notifications_* tools: memory_add, memory_list, memory_search,
// memory_prime, memory_stats, notifications_list, notifications_stats,
// notifications_mark_read.
func memoryToolDefs() []toolDef {
	return []toolDef{
		{
			name:        "sin_memory_add",
			description: "Add a long-term project memory (insight, project, tags). Used by orchestrator agents via prime context.",
			handler:     handleMemoryAdd,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"insight"},
				"properties": map[string]any{
					"insight": map[string]any{"type": "string"},
					"project": map[string]any{"type": "string"},
					"tags":    map[string]any{"type": "string", "description": "Comma-separated"},
				},
			},
		},
		{
			name:        "sin_memory_list",
			description: "List project memories (filter by project/tag)",
			handler:     handleMemoryList,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"project": map[string]any{"type": "string"},
					"tag":     map[string]any{"type": "string"},
				},
			},
		},
		{
			name:        "sin_memory_search",
			description: "Semantic search (uses NIM embeddings if SIN_NIM_API_KEY is set; substring fallback otherwise)",
			handler:     handleMemorySearch,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query":   map[string]any{"type": "string"},
					"project": map[string]any{"type": "string"},
					"top":     map[string]any{"type": "integer", "default": 10},
				},
			},
		},
		{
			name:        "sin_memory_prime",
			description: "Print top-K relevant memories for an LLM prompt (markdown formatted, ready to inject)",
			handler:     handleMemoryPrime,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query":   map[string]any{"type": "string"},
					"project": map[string]any{"type": "string"},
					"top":     map[string]any{"type": "integer", "default": 10},
				},
			},
		},
		{
			name:        "sin_memory_stats",
			description: "Memory DB statistics (total, links, embeddings, embedder status)",
			handler:     handleMemoryStats,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_notifications_list",
			description: "List recent non-dismissed notifications (JSON, top 50)",
			handler:     handleNotificationsList,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{"type": "integer", "default": 50},
				},
			},
		},
		{
			name:        "sin_notifications_stats",
			description: "Notification statistics (total, unread, by type)",
			handler:     handleNotificationsStats,
			schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name:        "sin_notifications_mark_read",
			description: "Mark a notification as read by ID",
			handler:     handleNotificationsMarkRead,
			schema: map[string]any{
				"type":     "object",
				"required": []string{"id"},
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
			},
		},
	}
}
