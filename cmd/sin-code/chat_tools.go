// SPDX-License-Identifier: MIT
// Purpose: builtin local toolset for `sin-code chat` (issue #44). Names
// match the permission default matrix: sin_read/sin_write/sin_edit allow,
// sin_bash ask.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
)

const (
	maxReadBytes  = 64 * 1024
	maxToolOutput = 32 * 1024
	bashTimeout   = 120 * time.Second
	maxSearchHits = 100
)

type agentloopToolSpecAlias = agentloop.ToolSpec

func builtinSpecs() []agentloopToolSpecAlias {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	specs := []agentloopToolSpecAlias{
		{Name: "sin_read", Description: "Read a file (UTF-8, capped at 64KB).",
			InputSchema: obj(map[string]any{"path": str("file path")}, "path")},
		{Name: "sin_write", Description: "Atomically write content to a file, creating parent dirs.",
			InputSchema: obj(map[string]any{"path": str("file path"), "content": str("full file content")}, "path", "content")},
		{Name: "sin_edit", Description: "Replace the first exact occurrence of old with new in a file.",
			InputSchema: obj(map[string]any{"path": str("file path"), "old": str("exact text to replace"), "new": str("replacement text")}, "path", "old", "new")},
		{Name: "sin_bash", Description: "Run a shell command in the workspace (120s timeout).",
			InputSchema: obj(map[string]any{"command": str("shell command")}, "command")},
		{Name: "sin_search", Description: "Search files for a substring; returns file:line matches.",
			InputSchema: obj(map[string]any{"pattern": str("substring to search"), "dir": str("directory (default .)")}, "pattern")},
	}
	return append(specs, extraSpecs()...)
}

func builtinTool(ctx context.Context, name string, args map[string]any) (string, error) {
	switch name {
	case "sin_read":
		return toolRead(argStr(args, "path"))
	case "sin_write":
		return toolWrite(argStr(args, "path"), argStr(args, "content"))
	case "sin_edit":
		return toolEdit(argStr(args, "path"), argStr(args, "old"), argStr(args, "new"))
	case "sin_bash":
		return toolBash(ctx, argStr(args, "command"))
	case "sin_search":
		return toolSearch(argStr(args, "pattern"), argStr(args, "dir"))
	default:
		return extraTool(ctx, name, args)
	}
}

func argStr(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return v
}

func toolRead(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("sin_read: path required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > maxReadBytes {
		return string(data[:maxReadBytes]) + "\n[... truncated]", nil
	}
	return string(data), nil
}

func toolWrite(path, content string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("sin_write: path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp := path + ".sin-tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), path), nil
}

func toolEdit(path, old, new string) (string, error) {
	if path == "" || old == "" {
		return "", fmt.Errorf("sin_edit: path and old required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if !strings.Contains(content, old) {
		return "", fmt.Errorf("sin_edit: old text not found in %s", path)
	}
	updated := strings.Replace(content, old, new, 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", err
	}
	return "edited " + path, nil
}

func toolBash(ctx context.Context, command string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("sin_bash: command required")
	}
	cctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if len(text) > maxToolOutput {
		text = text[:maxToolOutput] + "\n[... truncated]"
	}
	if err != nil {
		return fmt.Sprintf("exit error: %v\n%s", err, text), nil
	}
	return text, nil
}

func toolSearch(pattern, dir string) (string, error) {
	if pattern == "" {
		return "", fmt.Errorf("sin_search: pattern required")
	}
	if dir == "" {
		dir = "."
	}
	var hits []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || len(hits) >= maxSearchHits {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil || len(data) > 2*1024*1024 {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, pattern) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
				if len(hits) >= maxSearchHits {
					return nil
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(hits) == 0 {
		return "no matches", nil
	}
	return strings.Join(hits, "\n"), nil
}
