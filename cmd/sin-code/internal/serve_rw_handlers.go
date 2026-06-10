// SPDX-License-Identifier: MIT
// Purpose: MCP handlers for sin_read, sin_write, sin_edit. Call the internal
// readFile/writeFileAtomic/applyEdit functions directly (no subprocess),
// making the edit loop hot path as fast as possible.
// Docs: cmd/sin-code/internal/serve_rw_handlers.doc.md
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

func handleRead(ctx context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	mode := stringArg(args, "mode", "hashline")
	offset := intArg(args, "offset", 1)
	limit := intArg(args, "limit", 0)
	maxBytes := int64(intArg(args, "max_bytes", int(readDefaultMaxBytes)))

	result, err := readFile(absPath, mode, offset, limit, maxBytes)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(result, "", "  ")
	return string(out), err
}

func handleWrite(ctx context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	content, hasContent := args["content"].(string)
	if path == "" || !hasContent {
		return "", fmt.Errorf("path and content are required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	result, err := writeFileAtomic(absPath, content, writeOpts{
		validate: !boolArg(args, "no_validate"),
		backup:   boolArg(args, "backup"),
		mkdir:    boolArg(args, "mkdir"),
	})
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(result, "", "  ")
	return string(out), err
}

func handleEdit(ctx context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	req := editRequest{
		Anchor:     stringArg(args, "anchor", ""),
		EndAnchor:  stringArg(args, "end_anchor", ""),
		NewText:    stringArg(args, "new_text", ""),
		OldString:  stringArg(args, "old_string", ""),
		NewString:  stringArg(args, "new_string", ""),
		ReplaceAll: boolArg(args, "replace_all"),
		Insert:     stringArg(args, "insert", ""),
		Delete:     boolArg(args, "delete"),
		DryRun:     boolArg(args, "dry_run"),
		Validate:   !boolArg(args, "no_validate"),
		Drift:      intArg(args, "drift", DefaultDriftWindow),
	}
	result, err := applyEdit(absPath, req)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(result, "", "  ")
	return string(out), err
}

func stringArg(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok && v != "" {
		return v
	}
	return def
}

func intArg(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

func boolArg(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}
