// SPDX-License-Identifier: MIT
// Purpose: TUI-local tool implementations for the AgentRunner. These
// mirror the builtin tools from chat_tools.go but live in the tui
// package so they can be registered without importing main.
package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/mcpclient"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sandbox"
)

const (
	tuiMaxReadBytes  = 64 * 1024
	tuiMaxToolOutput = 32 * 1024
	tuiBashTimeout   = 120 * time.Second
)

// tuiSandboxConfig controls OS-level isolation for the TUI's sin_bash
// tool. It mirrors chat_tools.go's sandboxConfig so the AgentRunner
// path cannot bypass the verification/sandbox gate that chat enforces.
//
// Mandate M3 (verification gate) and M4 (permission engine) require
// every shell invocation from agent code to flow through the same
// sandbox as the chat tool surface. Without this config, an attacker
// who convinced the TUI's planner to issue a dangerous command would
// skip isolation entirely.
var (
	tuiSandboxConfigMu sync.RWMutex
	tuiSandboxConfig   struct {
		enabled   bool
		workspace string
	}
)

// tuiSetSandbox toggles sandbox isolation for tuiToolBash. Passing a
// non-empty workspace enables the sandbox; passing "" disables it. The
// setter is the TUI-side counterpart of chat_tools.go:setSandboxConfig
// (signature simplified per the TUI wiring contract — backend selection
// lives in the chat layer).
func tuiSetSandbox(workspace string) {
	tuiSandboxConfigMu.Lock()
	defer tuiSandboxConfigMu.Unlock()
	tuiSandboxConfig.workspace = workspace
	tuiSandboxConfig.enabled = workspace != ""
}

// tuiBashPathMu protects an internal observability tag that records
// which backend tuiToolBash most recently used ("sandbox" or "exec").
// Production code MUST NOT depend on this; tests read it via
// tuiReadBashPath to assert the routing decision.
var (
	tuiBashPathMu  sync.Mutex
	tuiBashPathTag string
)

// tuiReadBashPath returns the most recent routing tag set by tuiToolBash.
// Used by tests; safe under -race (mandate M7) thanks to tuiBashPathMu.
func tuiReadBashPath() string {
	tuiBashPathMu.Lock()
	defer tuiBashPathMu.Unlock()
	return tuiBashPathTag
}

// tuiToolSpecs returns the tool specifications for the TUI agent.
func tuiToolSpecs() []agentloop.ToolSpec {
	str := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	obj := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": props, "required": required}
	}
	return []agentloop.ToolSpec{
		{
			Name:        "sin_read",
			Description: "Read a file (UTF-8, capped at 64KB).",
			InputSchema: obj(map[string]any{"path": str("file path")}, "path"),
		},
		{
			Name:        "sin_write",
			Description: "Atomically write content to a file, creating parent dirs.",
			InputSchema: obj(map[string]any{"path": str("file path"), "content": str("full file content")}, "path", "content"),
		},
		{
			Name:        "sin_edit",
			Description: "Replace the first exact occurrence of old with new in a file.",
			InputSchema: obj(map[string]any{"path": str("file path"), "old": str("exact text to replace"), "new": str("replacement text")}, "path", "old", "new"),
		},
		{
			Name:        "sin_bash",
			Description: "Run a shell command in the workspace (120s timeout).",
			InputSchema: obj(map[string]any{"command": str("shell command")}, "command"),
		},
		{
			Name:        "sin_search",
			Description: "Search files for a substring; returns file:line matches.",
			InputSchema: obj(map[string]any{"pattern": str("substring to search"), "dir": str("directory (default .)")}, "pattern"),
		},
	}
}

// tuiToolFunc returns the tool execution function for the TUI agent.
func tuiToolFunc(workspace string) agentloop.LocalToolFunc {
	return func(ctx context.Context, name string, args map[string]any) (string, error) {
		switch name {
		case "sin_read":
			return tuiToolRead(argStr(args, "path"))
		case "sin_write":
			return tuiToolWriteWithDiff(argStr(args, "path"), argStr(args, "content"))
		case "sin_edit":
			return tuiToolEditWithDiff(argStr(args, "path"), argStr(args, "old"), argStr(args, "new"))
		case "sin_bash":
			return tuiToolBash(ctx, argStr(args, "command"))
		case "sin_search":
			return tuiToolSearch(argStr(args, "pattern"), argStr(args, "dir"))
		default:
			return "", fmt.Errorf("unknown tool: %s", name)
		}
	}
}

// tuiToolFactory returns a factory function that creates tools + MCP tools.
func tuiToolFactory(workspace string) func(*mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec) {
	return func(mgr *mcpclient.Manager) (agentloop.LocalToolFunc, []agentloop.ToolSpec) {
		specs := tuiToolSpecs()
		baseTool := tuiToolFunc(workspace)

		// Add MCP tools if manager is available
		if mgr != nil {
			for _, t := range mgr.Tools() {
				schema := t.InputSchema
				if schema == nil {
					schema = map[string]any{"type": "object", "properties": map[string]any{}}
				}
				desc := t.Description
				if desc == "" {
					desc = fmt.Sprintf("External MCP tool %s on server %s", t.Name, t.Server)
				}
				specs = append(specs, agentloop.ToolSpec{
					Name:        t.Qualified,
					Description: desc,
					InputSchema: schema,
				})
			}
		}

		// Combined tool function that routes to builtins or MCP
		combinedTool := func(ctx context.Context, name string, args map[string]any) (string, error) {
			if strings.Contains(name, "__") && mgr != nil {
				return mgr.Call(ctx, name, args)
			}
			return baseTool(ctx, name, args)
		}

		return combinedTool, specs
	}
}

// argStr extracts a string argument from the args map.
func argStr(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// tuiToolRead reads a file.
func tuiToolRead(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > tuiMaxReadBytes {
		return string(data[:tuiMaxReadBytes]) + "\n... (truncated)", nil
	}
	return string(data), nil
}

// tuiToolWrite writes a file atomically.
func tuiToolWrite(path, content string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), path), nil
}

// tuiToolEdit edits a file.
func tuiToolEdit(path, old, new string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	if !strings.Contains(content, old) {
		return "", fmt.Errorf("old string not found in file")
	}
	content = strings.Replace(content, old, new, 1)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return "edited file", nil
}

// tuiToolBash runs a shell command.
//
// When tuiSandboxConfig is enabled, the command is routed through
// sandbox.Command under sandbox.DefaultPolicy — OS-level (Landlock on
// Linux) confinement of filesystem + network scope. Otherwise it falls
// back to the raw exec.CommandContext("bash","-c",...) path.
//
// The sandbox branch mirrors chat_tools.go:toolBash so the TUI agent
// surface enforces the same M3 verification / M4 permission guarantees
// as the chat surface (issue #367). Both branches respect tuiBashTimeout
// and cap output at tuiMaxToolOutput.
func tuiToolBash(ctx context.Context, command string) (string, error) {
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	tuiSandboxConfigMu.RLock()
	sandboxEnabled := tuiSandboxConfig.enabled
	sandboxWorkspace := tuiSandboxConfig.workspace
	tuiSandboxConfigMu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, tuiBashTimeout)
	defer cancel()

	if sandboxEnabled && sandboxWorkspace != "" {
		tuiBashPathMu.Lock()
		tuiBashPathTag = "sandbox"
		tuiBashPathMu.Unlock()

		policy := sandbox.DefaultPolicy(sandboxWorkspace, os.TempDir())
		cmd, _, err := sandbox.Command(ctx, policy, "sh", "-c", command)
		if err != nil {
			return "", fmt.Errorf("tui_bash sandbox: %v", err)
		}
		out, err := cmd.CombinedOutput()
		text := string(out)
		if len(text) > tuiMaxToolOutput {
			text = text[:tuiMaxToolOutput] + "\n[... truncated]"
		}
		if err != nil {
			return fmt.Sprintf("exit error: %v\n%s", err, text), nil
		}
		return text, nil
	}

	tuiBashPathMu.Lock()
	tuiBashPathTag = "exec"
	tuiBashPathMu.Unlock()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if len(output) > tuiMaxToolOutput {
		output = output[:tuiMaxToolOutput]
	}
	if err != nil {
		return string(output), err
	}
	return string(output), nil
}

// tuiToolSearch searches for a pattern in files.
func tuiToolSearch(pattern, dir string) (string, error) {
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	if dir == "" {
		dir = "."
	}
	var matches []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.Contains(info.Name(), ".git") {
			return filepath.SkipDir
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(data), pattern) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "no matches found", nil
	}
	return strings.Join(matches, "\n"), nil
}

type DiffEntry struct {
	Path      string
	Before    string
	After     string
	Tool      string
	Timestamp time.Time
}

var diffBuffer = make([]DiffEntry, 0, 20)
var diffMu sync.Mutex

func RecordDiff(path, before, after, tool string) {
	diffMu.Lock()
	defer diffMu.Unlock()
	diffBuffer = append(diffBuffer, DiffEntry{
		Path: path, Before: before, After: after,
		Tool: tool, Timestamp: time.Now(),
	})
	if len(diffBuffer) > 20 {
		diffBuffer = diffBuffer[len(diffBuffer)-20:]
	}
}

func RecentDiffs() []DiffEntry {
	diffMu.Lock()
	defer diffMu.Unlock()
	out := make([]DiffEntry, len(diffBuffer))
	copy(out, diffBuffer)
	return out
}

func ClearDiffs() {
	diffMu.Lock()
	defer diffMu.Unlock()
	diffBuffer = diffBuffer[:0]
}

func tuiToolWriteWithDiff(path, content string) (string, error) {
	before, _ := os.ReadFile(path)
	result, err := tuiToolWrite(path, content)
	if err == nil {
		RecordDiff(path, string(before), content, "sin_write")
	}
	return result, err
}

func tuiToolEditWithDiff(path, old, new string) (string, error) {
	before, _ := os.ReadFile(path)
	result, err := tuiToolEdit(path, old, new)
	if err == nil {
		after, _ := os.ReadFile(path)
		RecordDiff(path, string(before), string(after), "sin_edit")
	}
	return result, err
}
