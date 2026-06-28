---
name: skill-code-mcp-builder
description: Use when user says 'build MCP server', 'scaffold MCP', 'new MCP tool', 'MCP server builder', 'create MCP'. Meta-skill that scaffolds new MCP servers in python-fastmcp, node-mcp, or go-mcp. Provides tools for scaffold, template_list, add_tool, test, register, validate, publish, and audit.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
  sources: "OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/mcp-server-builder"
required_tools:
  - sin_write
  - sin_edit
lifecycle: external
---

# skill-code-mcp-builder

## Overview

Scaffold, extend, validate, publish, and audit MCP servers using the canonical OpenSIN-Code pattern: `pyproject.toml` + `src/<pkg>/mcp_server.py` + `tests/` + `*.doc.md` + `skill-code-ceo-audit.yml`.

## When to Use

- "Scaffold a new MCP server"
- "Create a new MCP tool"
- "Generate MCP project"
- "Add a tool to my existing MCP server"
- "Validate / publish / audit my MCP server"

## When NOT to Use

- Consuming an existing MCP server (use `mcpclient` / `opencode.json` config).
- Writing application code unrelated to MCP.

## Core Process

```
SCAFFOLD → ADD TOOLS → TEST → VALIDATE → AUDIT → PUBLISH → REGISTER
```

1. Choose template (`python-fastmcp`, `node-mcp`, `go-mcp`).
2. Scaffold the project with initial tools.
3. Add or refine tools.
4. Generate tests.
5. Validate structure and types.
6. Run CEO audit.
7. Publish and register in `opencode.json`.

## Templates

| Template | Stack | Entry point |
|---|---|---|
| `python-fastmcp` | Python + FastMCP | `src/<pkg>/mcp_server.py` |
| `node-mcp` | Node.js + `@modelcontextprotocol/sdk` | `src/index.js` |
| `go-mcp` | Go + `github.com/modelcontextprotocol/go-sdk` | `main.go` |

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll scaffold manually." | The canonical pattern ensures consistency and auditability. |
| "Tests are optional." | Every tool needs a generated test. |
| "I'll skip audit." | MCP servers must pass CEO audit before publish. |

## Red Flags

- Scaffolding without tests.
- Publishing without validation.
- Skipping CEO audit.
- Not registering the server in `opencode.json`.

## Verification

- [ ] Template and tools chosen.
- [ ] Project scaffolded.
- [ ] Tests generated for each tool.
- [ ] Validation passes.
- [ ] CEO audit passes.
- [ ] Published/registered successfully.
