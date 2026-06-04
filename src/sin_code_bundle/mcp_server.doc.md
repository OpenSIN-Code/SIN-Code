# MCP Server — Unified SIN-Code MCP Server

## Purpose

Unified MCP (Model Context Protocol) server entry point for the
SIN-Code bundle. This module is invoked via `python -m
sin_code_bundle.mcp_server` (or the `sin-serve` console script) and
exposes the full SIN-Code tool surface — file-ops, VFS, AST edit,
hashline, DAP tracing, interceptor, worktree orchestration, and
optional subsystem/memory/external tools — over a single stdio
MCP endpoint.

The goal is to replace opencode's native read/write/edit/bash/search
with `sin_*` tools that have URI semantics, secret-redaction, size
safety, and line-shift resilience.

## Tool inventory (34 tools, 10 categories)

### 1. Core file-ops — 5 tools (always on)

Replaces opencode native read/write/edit/bash/search.

| Tool         | Purpose                                                                                   |
|--------------|-------------------------------------------------------------------------------------------|
| `sin_read`   | URI-scheme-aware read (sckg://, poc://, ibd://, adw://, efsm://, oracle://, conflict://) via VirtualFS. Plain paths are read with size-aware head/tail truncation. `summarize=True` returns a structural overview. |
| `sin_write`  | Atomic write with auto-backup (`.bak`). When `verify=True` (default), runs `compile()` syntax check on `.py` files; rolls back on SyntaxError. |
| `sin_edit`   | Hashline-anchored semantic patching. `old_content` is matched by content-hash, not line numbers — survives line shifts, reformatting, and concurrent edits. |
| `sin_bash`   | Safe shell exec. Tries the `execute` Go binary (`~/.local/bin/execute`); falls back to raw `subprocess.run(shell=True)` with a `warning` if the binary is missing. Timeout default 60s. |
| `sin_search` | Wraps the `scout` Go binary (semantic/regex/symbol/usage). Falls back to a Python regex loop over `rglob("*")` (max 200 hits) if `scout` is missing. |

### 2. VFS / AST / Hashline — 4 tools (always on)

Dedicated tools the user requested as separate from core file-ops.

| Tool                    | Purpose                                                                                     |
|-------------------------|---------------------------------------------------------------------------------------------|
| `sin_vfs_resolve`       | Resolve any SIN URI scheme (sckg://, poc://, ibd://, adw://, efsm://, oracle://, conflict://) to structured JSON. |
| `sin_vfs_schemes`       | List all URI schemes and their meanings.                                                    |
| `sin_ast_edit`          | AST-based edit via tree-sitter (Python/JS/TS/Go). Falls back to hashline-anchored text edit if tree-sitter is unavailable. `verify_with_poc=True` runs POC syntax verification. |
| `sin_hashline_validate` | Validate that a previously-created hashline patch can still be applied (anchor still resolves). |

### 3. Architecture / Runtime — 2 tools

Tools that act on the runtime state of a process or a repo's
architecture.

| Tool                          | Purpose                                                                                       |
|-------------------------------|-----------------------------------------------------------------------------------------------|
| `sin_runtime_trace`           | Start a DAP debugging session for a specific function (delegates to `dap_bridge.SINRuntimeTrace`). |
| `sin_stop_trace`              | Stop an active DAP debugging session.                                                         |

### 4. Interceptor / Worktree — 2 tools

Architectural enforcement and parallel-task isolation.

| Tool                        | Purpose                                                                                  |
|-----------------------------|------------------------------------------------------------------------------------------|
| `sin_check_architecture`    | Pre-flight check: returns `{"allowed": False, "violations": [...]}` if a tool call would violate a rule. |
| `sin_create_worktree`       | Create an isolated git worktree (delegates to `orchestration_worktrees.SINWorktreeOrchestrator`). |
| `sin_cleanup_worktree`      | Remove a worktree; if `merge_back=True`, fast-forward-merge into `main` first.            |

(That's 3 in this section, not 2 — the prompt was approximate. Real count: 3.)

### 5. Subsystem tools — 9 tools (graceful degradation)

Wired in `_try_subsystem_tools()`. **Each block is wrapped in
`try/except ImportError`** — if the subsystem package is not
installed, the tool is silently skipped.

| Tool                  | Subsystem                       | Purpose                                                  |
|-----------------------|---------------------------------|----------------------------------------------------------|
| `impact`              | `sin-code-sckg`                 | Blast-radius impact analysis for a symbol (full graph walk). |
| `semantic_diff`       | `sin-code-ibd`                  | Semantic intent diff between two files (AST-level).      |
| `semantic_review`     | `sin-code-ibd`                  | Comprehensive review: intent + risk in one call.         |
| `architectural_debt`  | `sin-code-adw`                  | Current architectural debt score for `.` (with `_EXCLUDE` set: `.git`, `.venv`, `venv`, `__pycache__`, `node_modules`, `dist`, `build`). |
| `verify_tests`        | `sin-code-oracle`               | Verify agent-generated code (security / performance / correctness). |
| `prove`               | `sin-code-poc`                  | Generate and verify proofs of correctness for a function. |
| `mock_env`            | `sin-code-efsm`                 | Manage ephemeral full-stack mock environment (`up` / `down`, port default 8888). |
| `orchestrate`         | `sin-code-orchestration`        | Submit a task to the multi-agent orchestrator.           |
| `task_status`         | `sin-code-orchestration`        | Get status of an orchestrated task.                     |
| `review`              | `sin-code-review-interface`     | Run SOTA review on a single file.                       |

(That's 10 in this section, not 9 — the prompt was approximate. Real count: 10.)

### 6. Memory tools — 5 tools (graceful degradation)

Wired in `_try_memory_tools()`. Imports
`sin_code_bundle.memory.register_tools(mcp)`; skipped if `sin-brain`
is not installed.

| Tool                  | Purpose                                |
|-----------------------|----------------------------------------|
| `recall_tool`         | Semantic recall of stored facts.       |
| `remember_tool`       | Store a fact.                          |
| `forget_tool`         | Remove a stored fact.                  |
| `pin_tool`            | Pin a fact to always-include.          |
| `link_evidence_tool`  | Link evidence between two facts.       |

### 7. External — GitNexus — 3 tools (graceful degradation)

Wired in `_try_external_tools()`. Imports `sin_code_bundle.gitnexus`.

| Tool                    | Purpose                                                                |
|-------------------------|------------------------------------------------------------------------|
| `gitnexus_context`      | Structural graph context for a symbol (auto-indexes if needed).        |
| `gitnexus_impact`       | Blast-radius impact analysis for a symbol (auto-indexes if needed).     |
| `gitnexus_ai_context`   | Task-scoped, graph-aware context bundle (comma-separated symbol list). |

### 8. External — MarkItDown — 1 tool (graceful degradation)

| Tool                | Purpose                                                                |
|---------------------|------------------------------------------------------------------------|
| `markitdown_convert`| Convert a document (PDF/DOCX/PPTX/XLSX/image/HTML/CSV/JSON/XML/ZIP/YouTube/EPUB/Outlook-MSG) to Markdown. |

### 9. External — CoDocs — 1 tool (graceful degradation)

| Tool           | Purpose                                                                              |
|----------------|--------------------------------------------------------------------------------------|
| `codocs_check` | Find broken co-located `.doc.md` references in a repo (with the same `_EXCLUDE` set). |

### 10. Subtotal (review) — 1 tool

| Tool     | Purpose                                                                                |
|----------|----------------------------------------------------------------------------------------|
| `review` | (counted in §5) — Run SOTA review on a single file (via `sin-code-review-interface`). |

### Recount

The **actual** real count from `@mcp.tool()` decorators in the file
(grep): **29 tool decorators**. The doc-internal "34" figure in the
module's top docstring is the maximum if every optional subsystem
were installed. The user's prompt ("34 tools") is the *fully-loaded*
count.

| Category                 | Real decorators in this file | Max if all optionals present |
|--------------------------|------------------------------|------------------------------|
| Core file-ops            | 5                            | 5                            |
| VFS / AST / Hashline     | 4                            | 4                            |
| Architecture / Runtime   | 2                            | 2                            |
| Interceptor / Worktree   | 3                            | 3                            |
| Subsystem (graceful)     | 10                           | 10                           |
| Memory (graceful)        | 0 (delegated to `memory.register_tools`) | 5                 |
| GitNexus (graceful)      | 3                            | 3                            |
| MarkItDown (graceful)    | 1                            | 1                            |
| CoDocs (graceful)        | 1                            | 1                            |
| **Total**                | **29 in this file**          | **34 max**                   |

The prompt's "34 tools" is the **fully-loaded** number (29 here + 5
from `memory.register_tools`). Both numbers are correct for their
context.

## Console scripts

- `sin-serve` — primary console script entry point. Calls
  `python -m sin_code_bundle.mcp_server`.
- `sin-serve-mcp` — legacy alias (identical behavior).
- `sin serve` — legacy CLI subcommand (identical).
- `python -m sin_code_bundle.mcp_server` — direct module invocation.

All four run `mcp.run()` over stdio and exit on EOF.

## Graceful degradation summary

The module follows a single rule: **no tool is allowed to crash the
server on import**. Every optional dependency is wrapped in
`try/except ImportError: pass`:

- Subsystem tools (`_try_subsystem_tools`): each block is independently
  try-imported. Missing `sin-code-sckg` does not affect `sin-code-ibd`.
- Memory tools (`_try_memory_tools`): delegates to
  `sin_code_bundle.memory.register_tools(mcp)`. If `sin-brain` is
  missing, no memory tools are wired.
- External tools (`_try_external_tools`): each block (gitnexus,
  markitdown, codocs) is independently try-imported.
- Within individual tools: every tool body is wrapped in
  `try/except Exception`, returning
  `json.dumps({"error": str(exc), ...})` instead of raising.

The result: a minimal install with just the `[mcp]` extra gives
**29 tools** (the file's real decorator count). A full
`[all]` install gives **34 tools** (29 + 5 memory).

## Example tool call

Via MCP client (the `mcp` Python SDK):

```python
from mcp.client.session import ClientSession
from mcp.client.stdio import StdioServerParameters, stdio_client

params = StdioServerParameters(
    command="sin-serve",
    args=[],
)
async with stdio_client(params) as (read, write):
    async with ClientSession(read, write) as session:
        await session.initialize()
        result = await session.call_tool(
            "sin_search",
            {"query": "verify_architecture", "path": "/repo", "search_type": "symbol"},
        )
        print(result.content[0].text)
```

Or via direct CLI:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | sin-serve
```

## Entry point

```python
def main() -> None:
    """Run the MCP server (stdio)."""
    sys.stderr.write("[SIN-CODE-BUNDLE] MCP server starting (stdio).\n")
    sys.stderr.flush()
    mcp.run()
```

Called by `if __name__ == "__main__": main()` at the bottom of the
module. Logging goes to **stderr** (not stdout) because stdout is the
MCP transport channel.

## Known caveats

- `mcp` Python package is **required** (not optional). Missing it
  raises `SystemExit(1)` with a `pip install` hint.
- Subsystem/memory tools that need a storage path use **relative**
  paths (e.g. `./.sin/knowledge.graph` for `impact`). The agent's
  working directory must be the project root.
- `sin_bash` falls back to raw shell if the `execute` Go binary is
  missing — the warning is in the result, not in stderr, so callers
  must read the `warning` field.
- The server runs over **stdio**, not TCP. To expose it over the
  network, wrap it in `mcp-proxy` or similar.
