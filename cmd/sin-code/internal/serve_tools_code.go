// SPDX-License-Identifier: MIT
// Purpose: serve — tool definitions for code-intelligence and file-I/O tools.
// sin-debt: shrink, upgrade: consolidate when serve handlers are refactored
package internal

// codeToolDefs returns the tool definitions for code-intelligence and
// file-I/O tools: discover, execute, map, grasp, scout, harvest, index,
// read, write, edit.
func codeToolDefs() []toolDef {
	return []toolDef{
		{
			name:        "sin_discover",
			description: "Discover files with relevance scoring, pattern matching, and dependency analysis",
			handler:     handleDiscover,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Directory to search"},
					"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g. **/*.py)"},
					"format":  map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
					"limit":   map[string]any{"type": "integer", "default": 100},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_execute",
			description: "Execute shell commands safely with secret redaction, timeout, and error analysis",
			handler:     handleExecute,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string", "description": "Command to execute"},
					"timeout": map[string]any{"type": "integer", "default": 60},
					"format":  map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"command"},
			},
		},
		{
			name:        "sin_map",
			description: "Map code architecture with dependency graphs, entry points, and hot paths",
			handler:     handleMap,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Project root"},
					"action": map[string]any{"type": "string", "default": "map"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_grasp",
			description: "Deep code understanding for a single file — structure, dependencies, usage",
			handler:     handleGrasp,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "File to analyze"},
					"format": map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_scout",
			description: "Search code with regex, semantic, symbol, and usage search",
			handler:     handleScout,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":       map[string]any{"type": "string", "description": "Search query"},
					"path":        map[string]any{"type": "string", "default": "."},
					"search_type": map[string]any{"type": "string", "enum": []string{"regex", "semantic", "symbol", "usage"}, "default": "regex"},
					"format":      map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"query"},
			},
		},
		{
			name:        "sin_harvest",
			description: "Fetch URLs with caching, structure extraction, and change detection",
			handler:     handleHarvest,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":     map[string]any{"type": "string", "description": "URL to fetch"},
					"method":  map[string]any{"type": "string", "default": "GET"},
					"timeout": map[string]any{"type": "integer", "default": 30},
					"format":  map[string]any{"type": "string", "enum": []string{"text", "json"}, "default": "json"},
				},
				"required": []string{"url"},
			},
		},
		{
			name:        "sin_index",
			description: "Manage persistent incremental code index (build, refresh, status, clear)",
			handler:     handleIndex,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{"type": "string", "enum": []string{"build", "refresh", "status", "clear"}, "default": "status"},
					"root":   map[string]any{"type": "string", "default": "."},
				},
			},
		},
		{
			name:        "sin_read",
			description: "Read files token-efficiently: hashline mode emits LINE:HASH anchors for sin_edit, outline mode returns structure only for large files, raw mode is offset/limit guarded. Always prefer over native read.",
			handler:     handleRead,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": "File to read"},
					"mode":      map[string]any{"type": "string", "enum": []string{"hashline", "raw", "outline"}, "default": "hashline"},
					"offset":    map[string]any{"type": "integer", "default": 1, "description": "1-based start line"},
					"limit":     map[string]any{"type": "integer", "default": 2000, "description": "Max lines"},
					"max_bytes": map[string]any{"type": "integer", "description": "Size guard for raw/hashline reads"},
				},
				"required": []string{"path"},
			},
		},
		{
			name:        "sin_write",
			description: "Write a file atomically (temp+fsync+rename) with syntax pre-validation (Go parse, JSON parse, bracket-balance elsewhere). A failed validation never touches disk. Always prefer over native write.",
			handler:     handleWrite,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": "Destination file"},
					"content":     map[string]any{"type": "string", "description": "Full file content"},
					"no_validate": map[string]any{"type": "boolean", "default": false},
					"backup":      map[string]any{"type": "boolean", "default": false, "description": "Keep .bak of previous content"},
					"mkdir":       map[string]any{"type": "boolean", "default": false, "description": "Create parent directories"},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			name:        "sin_edit",
			description: "Surgical file edit, three addressing modes. Symbol mode (preferred for whole definitions): pass symbol=NAME to replace/delete/insert around an entire function/class/struct located via AST (go/ast, tree-sitter, or structural engine — ambiguity fails with candidates). Anchor mode: LINE:HASH anchors from sin_read, drift-tolerant. String mode: old_string/new_string with ambiguity detection. Result is syntax-validated and written atomically. Always prefer over native edit.",
			handler:     handleEdit,
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": "File to edit"},
					"symbol":      map[string]any{"type": "string", "description": "Symbol name for AST-anchored edits, e.g. \"handleScout\" or \"Server.Start\""},
					"anchor":      map[string]any{"type": "string", "description": "Start anchor LINE:HASH from sin_read"},
					"end_anchor":  map[string]any{"type": "string", "description": "End anchor for range edits (inclusive)"},
					"new_text":    map[string]any{"type": "string", "description": "Replacement/insertion text"},
					"old_string":  map[string]any{"type": "string", "description": "Exact string to replace (string mode)"},
					"new_string":  map[string]any{"type": "string", "description": "Replacement (string mode)"},
					"replace_all": map[string]any{"type": "boolean", "default": false},
					"insert":      map[string]any{"type": "string", "enum": []string{"before", "after"}, "description": "Insert relative to anchor/symbol"},
					"delete":      map[string]any{"type": "boolean", "default": false, "description": "Delete anchored line/range/symbol"},
					"dry_run":     map[string]any{"type": "boolean", "default": false, "description": "Return diff without writing"},
					"no_validate": map[string]any{"type": "boolean", "default": false},
					"drift":       map[string]any{"type": "integer", "default": 25, "description": "Anchor drift tolerance"},
				},
				"required": []string{"path"},
			},
		},
	}
}
