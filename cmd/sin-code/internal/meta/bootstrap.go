// SPDX-License-Identifier: MIT
// Purpose: bootstrap_skill — self-extending meta-tool that lets the
// agent write, test, and register its own MCP skill servers (issue
// #51, AGENTS.md §8 v3.6.0). The output is a tiny Python
// `mcp_server.py` that speaks JSON-RPC stdio and registers the
// generated skill in the workspace's `.sin-code/mcp.json`.
//
// Mandate M4 is preserved by two layers:
//   1. The chat-tool wrapper (chat_tools.go) checks
//      `SIN_ALLOW_BOOTSTRAP=1` before delegating here. Without the
//      env var, the agent cannot self-modify.
//   2. permission_defaults.go adds a default `ask` rule for
//      `sin_bootstrap_skill` so the engine prompts the user (or
//      denies in headless mode) regardless of profile.
package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// nameRE is the strict name validator. Lowercase first letter, then
// lowercase/digit/underscore, total length 1..32. Mirrors the contract
// the issue pins down explicitly.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// ServerTemplate is the body of the generated `mcp_server.py`. It
// supports two modes:
//   --list-tools (CLI flag, used for the bootstrap smoke test)
//   stdin JSON-RPC (`tools/list`, `tools/call`) for normal use
const ServerTemplate = `#!/usr/bin/env python3
"""Auto-generated MCP server for the {{NAME}} skill (issue #51)."""
from __future__ import annotations

import argparse
import json
import sys
from typing import Any, Dict

TOOL_NAME = "{{NAME}}__execute"
TOOL_DESCRIPTION = "Execute a {{NAME}} skill action (auto-generated)."
TOOL_INPUT_SCHEMA: Dict[str, Any] = {
    "type": "object",
    "properties": {
        "action": {"type": "string", "description": "verb to run"},
        "args":   {"type": "object", "description": "action arguments"},
    },
    "required": ["action"],
}


def _execute(action: str, args: Dict[str, Any]) -> str:
    # Replace this stub with real skill logic.
    return f"{TOOL_NAME} ran action={action!r} args={args!r}"


def _tools_list() -> Dict[str, Any]:
    return {
        "tools": [
            {
                "name": TOOL_NAME,
                "description": TOOL_DESCRIPTION,
                "inputSchema": TOOL_INPUT_SCHEMA,
            }
        ]
    }


def _handle_request(req: Dict[str, Any]) -> Dict[str, Any]:
    method = req.get("method", "")
    rid = req.get("id")
    if method == "initialize":
        return {
            "jsonrpc": "2.0",
            "id": rid,
            "result": {
                "protocolVersion": "2024-11-05",
                "capabilities": {"tools": {"listChanged": False}},
                "serverInfo": {"name": "{{NAME}}", "version": "0.1.0"},
            },
        }
    if method == "tools/list":
        return {"jsonrpc": "2.0", "id": rid, "result": _tools_list()}
    if method == "tools/call":
        params = req.get("params", {}) or {}
        name = params.get("name", "")
        arguments = params.get("arguments", {}) or {}
        if name != TOOL_NAME:
            return {
                "jsonrpc": "2.0",
                "id": rid,
                "error": {"code": -32602, "message": f"unknown tool {name!r}"},
            }
        action = arguments.get("action", "")
        args = arguments.get("args", {}) or {}
        out = _execute(action, args)
        return {
            "jsonrpc": "2.0",
            "id": rid,
            "result": {"content": [{"type": "text", "text": out}]},
        }
    return {
        "jsonrpc": "2.0",
        "id": rid,
        "error": {"code": -32601, "message": f"method not implemented: {method!r}"},
    }


def _serve_stdio() -> int:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            resp = _handle_request(req)
        except json.JSONDecodeError as exc:
            resp = {
                "jsonrpc": "2.0",
                "id": None,
                "error": {"code": -32700, "message": f"bad json: {exc}"},
            }
        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()
    return 0


def main(argv: list[str]) -> int:
    p = argparse.ArgumentParser(prog="{{NAME}}")
    p.add_argument("--list-tools", action="store_true",
                   help="print tool list as JSON and exit")
    args = p.parse_args(argv)
    if args.list_tools:
        json.dump(_tools_list(), sys.stdout)
        sys.stdout.write("\n")
        return 0
    return _serve_stdio()


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
`

// ValidateName returns nil if `name` matches the bootstrap name
// contract; otherwise a descriptive error. Exported so chat_tools.go
// (or any other caller) can do early validation before invoking the
// rest of the bootstrap.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("bootstrap_skill: name required")
	}
	if !nameRE.MatchString(name) {
		return fmt.Errorf("bootstrap_skill: invalid name %q (must match ^[a-z][a-z0-9_]{0,31}$)", name)
	}
	return nil
}

// QualifiedName is the canonical tool namespace for the new skill.
// The MCP layer exposes it as `<name>__*`; bootstrap returns it
// unchanged so callers can register it directly.
func QualifiedName(name string) string { return name + "__*" }

// BootstrapSkill is the public entry point. It writes
// `<workspace>/.sin-code/skills/<name>/mcp_server.py`, runs the smoke
// test, and merges the entry into `<workspace>/.sin-code/mcp.json`.
// `spec` is currently unused but kept in the signature so the template
// can be parameterised later without breaking callers.
//
// Returns the qualified tool name on success. Any non-nil error
// means nothing was registered (atomic at the file-system level).
func BootstrapSkill(ctx context.Context, workspace, name, spec string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	// Skip silently when python3 is missing — bootstrap is a
	// development affordance, not a hard runtime dependency.
	if _, err := exec.LookPath("python3"); err != nil {
		return "", fmt.Errorf("bootstrap_skill: python3 not on PATH: %w", err)
	}
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		workspace = wd
	}
	dir := filepath.Join(workspace, ".sin-code", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	script := filepath.Join(dir, "mcp_server.py")
	body := renderTemplate(ServerTemplate, name, spec)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		return "", err
	}
	// Smoke test: --list-tools must succeed.
	list := exec.CommandContext(ctx, "python3", script, "--list-tools")
	out, err := list.CombinedOutput()
	if err != nil {
		// Best-effort rollback so a failed bootstrap never leaves
		// a half-registered skill on disk.
		_ = os.Remove(script)
		_ = os.Remove(dir)
		return "", fmt.Errorf("bootstrap_skill: smoke test failed: %w: %s", err, string(out))
	}
	var toolsResp struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(out, &toolsResp); err != nil {
		_ = os.Remove(script)
		_ = os.Remove(dir)
		return "", fmt.Errorf("bootstrap_skill: --list-tools produced invalid JSON: %w: %s", err, string(out))
	}
	if len(toolsResp.Tools) == 0 || toolsResp.Tools[0].Name == "" {
		_ = os.Remove(script)
		_ = os.Remove(dir)
		return "", fmt.Errorf("bootstrap_skill: --list-tools returned no tools")
	}
	// Merge into mcp.json.
	if err := mergeMCPConfig(workspace, name, script); err != nil {
		_ = os.Remove(script)
		_ = os.Remove(dir)
		return "", err
	}
	return QualifiedName(name), nil
}

func renderTemplate(tpl, name, _ string) string {
	// Name is already validated by ValidateName, so a simple
	// substitution is safe. The Go regexp check guarantees the name
	// can never contain characters that would let an attacker break
	// out of the Python string context.
	return strings.ReplaceAll(tpl, "{{NAME}}", name)
}

// mergeMCPConfig appends a stdio entry for `name` to
// `<workspace>/.sin-code/mcp.json`. Existing entries are preserved
// (a key collision with the same name overwrites the existing entry;
// all other entries are left untouched).
func mergeMCPConfig(workspace, name, scriptPath string) error {
	configPath := filepath.Join(workspace, ".sin-code", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}
	root := map[string]any{}
	if data, err := os.ReadFile(configPath); err == nil && len(data) > 0 {
		// Best-effort: a malformed mcp.json is a user error and
		// must not silently destroy their other servers. We surface
		// the parse error and refuse to merge.
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("bootstrap_skill: existing %s is not valid JSON: %w", configPath, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	servers[name] = map[string]any{
		"transport": "stdio",
		"command":   "python3",
		"args":      []string{scriptPath},
	}
	root["mcpServers"] = servers
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, out, 0o644)
}
