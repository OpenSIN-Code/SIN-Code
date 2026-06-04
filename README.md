# 🧠 SIN-Code Bundle

[![Python](https://img.shields.io/badge/python-3.11%2B-blue.svg)](https://www.python.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)
[![CI](https://img.shields.io/badge/CI-A%2B%20(100%2F100)-brightgreen)](#)
[![Version](https://img.shields.io/badge/Version-0.6.1-blue)](#)

**The universal semantic backend for AI coding agents.**

One CLI (`sin`) and one unified MCP server (`sin-serve` / `sin serve`) that
orchestrate **8 Python subsystems, 7 Go tools, and 5 external bridges** — giving
AI agents the signals they actually lack: structural knowledge, semantic diffs,
correctness proofs, ephemeral test environments, debt guardrails, and
persistent memory.

---

## ⚡ Why SIN-Code? (The Anti-Dreck Promise)

Most coding agents operate blind — fragile string replacements, blind bash
execution, no memory between sessions. SIN-Code replaces native agent tools
with semantically aware, verifiable alternatives.

| The Old Way (Dreck) | The SIN-Code Way |
|---|---|
| ❌ Fragile `str_replace` (whitespace errors, stale files) | ✅ `sin_edit`: Hashline-anchored, content-hash verified patching (line-shift resilient) |
| ❌ Blind `read` (dumps 2000 lines of context) | ✅ `sin_read`: Deep structural analysis OR semantic URIs (`sckg://module/auth/neighbors`) — size-safe with summarize mode |
| ❌ Dangerous `bash` (secret leaks, infinite loops) | ✅ `sin_bash`: Secret-redaction + strict timeouts + structured JSON (safety_check, retry_info) |
| ❌ Stateless amnesia (forgets user prefs every session) | ✅ `recall_tool`/`remember_tool`: Persistent 4-tier memory (SQLite+FTS5) via [sin-brain](https://github.com/OpenSIN-Code/SIN-Brain) |
| ❌ Architectural drift (agents spaghetti-fy the code) | ✅ `sin_check_architecture`: Pre-flight ADW rule enforcement — blocks hardcoded secrets, eval/exec, frontend-DB imports |
| ❌ Sequential exploration (one slow task at a time) | ✅ `sin_create_worktree`: Isolated git worktrees for parallel agent tasks without conflicts |
| ❌ Static analysis only (no runtime insight) | ✅ `sin_runtime_trace`: DAP debug session attachment (debugpy/dlv/node) |

---

## 🚀 Quickstart (2-5 minutes)

```bash
git clone https://github.com/OpenSIN-Code/SIN-Code-Bundle.git
cd SIN-Code-Bundle
bash install.sh        # Bootstraps 7 Go tools + Python bundle + MCP config + 5 external bridges
sin status            # Show what's installed
sin bootstrap /path/to/your/repo   # Initialize graphs, baselines, ledgers
sin-serve             # Start the unified MCP server (or: sin serve)
```

Flags: `--help`, `--dry-run`, `--verbose`, `--force`, `--skip-go`, `--skip-external`

Environment overrides:
```bash
SIN_CODE_BIN_DIR=~/custom-bin SIN_CODE_REPOS_DIR=~/my-repos bash install.sh
```

---

## 🛠️ Agent Usage Example

Instead of guessing file structures, agents query the Semantic Codebase
Knowledge Graph (SCKG) directly via stable URI schemes:

```json
// Agent tool call
{
  "name": "sin_read",
  "arguments": {
    "path": "sckg://module/auth-service/dependencies",
    "summarize": true
  }
}
```

The VFS resolver translates this into a structured JSON response of exact
module dependencies — saving hundreds of tokens and preventing hallucination.

To disable native opencode tools and force SIN-Code usage, add to
`~/.config/opencode/opencode.json`:

```json
{
  "tools": {
    "read": false, "write": false, "edit": false, "bash": false,
    "search": false, "find": false, "grep": false, "glob": false,
    "list": false, "webfetch": false, "task": false
  },
  "mcp": {
    "sin-code-bundle": {
      "type": "local",
      "command": ["sin", "serve"],
      "enabled": true
    }
  }
}
```

---

## 📦 Unified MCP Tool Inventory (34 Tools)

When `sin-serve` is running, agents get **34 tools**. Native opencode tools
should be **disabled** to enforce SIN-Code usage.

### Core File Operations (5) — Replace native read/write/edit/bash/search

| Tool | Replaces | What it does | Why better than native |
|---|---|---|---|
| `sin_read` | `read` | URI-aware file read with size-safety + summarize mode | Native dumps 10MB+ into context; sin_read truncates + supports `sckg://`, `poc://`, `ibd://`, `adw://`, `efsm://`, `oracle://`, `conflict://` URIs |
| `sin_write` | `write` | Atomic write + syntax pre-validation (.py/.ts/.js/.go) + auto-backup | Native creates half-written files on crash; sin_write is atomic + compiles before writing |
| `sin_edit` | `edit` | Hashline-anchored semantic patches (content-hash, not line numbers) | Native edit breaks on reformat/race; sin_edit survives line shifts |
| `sin_bash` | `bash` | Safe shell exec via Go `execute` binary | Native leaks secrets; sin_bash redacts tokens/keys + enforces timeout |
| `sin_search` | `search`/`find`/`grep`/`glob` | Scout (Go) + Python-regex fallback | Native has 4 separate tools; sin_search unifies all 4 with semantic mode |

### Virtual Filesystem — URI Schemes (2)

| Tool | What it does |
|---|---|
| `sin_vfs_resolve` | Resolve `sckg://`, `poc://`, `ibd://`, `adw://`, `efsm://`, `oracle://`, `conflict://` URIs to structured content |
| `sin_vfs_schemes` | List all available URI schemes |

### Code Structure — AST + Hashline (2)

| Tool | What it does |
|---|---|
| `sin_ast_edit` | Tree-sitter AST-based edit with POC verification (falls back to hashline) |
| `sin_hashline_validate` | Validate a previously-created hashline patch can still be applied |

### Architectural Enforcement (1)

| Tool | What it does |
|---|---|
| `sin_check_architecture` | Pre-flight ADW rule check — blocks hardcoded secrets, eval/exec, frontend-DB imports |

### Runtime Debugging — DAP (2)

| Tool | What it does |
|---|---|
| `sin_runtime_trace` | Start a DAP debug session for a function (debugpy for Python, dlv for Go, node --inspect for Node) |
| `sin_stop_trace` | Stop an active DAP session |

### Parallel Task Execution (2)

| Tool | What it does |
|---|---|
| `sin_create_worktree` | Create an isolated git worktree for parallel agent tasks |
| `sin_cleanup_worktree` | Clean up worktree (optionally merge back to main) |

### Subsystem Tools (10) — Require subsystem packages via `pip install -e ".[all]"`

| Tool | Subsystem | What it does |
|---|---|---|
| `impact` | sin_code_sckg | Blast-radius impact analysis for a symbol |
| `semantic_diff` | sin_code_ibd | Semantic intent diff between two files |
| `semantic_review` | sin_code_ibd | Intent + risk score in one call |
| `architectural_debt` | sin_code_adw | Current architectural debt score (god modules, circular imports, hot paths without tests) |
| `verify_tests` | sin_code_oracle | Verify agent-generated code (security/perf/correctness — OWASP Top 10, CWE Top 25) |
| `prove` | sin_code_poc | Generate and verify proofs of correctness (Hoare-style pre/post-conditions) |
| `mock_env` | sin_code_efsm | Manage ephemeral full-stack mock environment (Postgres + Redis + API server in 10s) |
| `orchestrate` | sin_code_orchestration | Submit a task to the multi-agent orchestrator (dependency-graph aware) |
| `task_status` | sin_code_orchestration | Get status of an orchestrated task |
| `review` | sin_code_review_interface | SOTA review on a single file (diff + debt + tests + style) |

### Memory Tools (5) — Require `sin-brain` via `pip install -e ".[memory]"`

| Tool | What it does |
|---|---|
| `recall_tool` | Search memory tiers (recall/archival/graph) — 4-tier SQLite+FTS5 storage |
| `remember_tool` | Persist a memory (decision/convention/fix/pitfall/preference) |
| `forget_tool` | Delete a memory entry by id |
| `pin_tool` | Pin a memory entry (never evicted) |
| `link_evidence_tool` | Attach a subsystem verdict to a memory |

### External Bridges (5)

| Tool | Source | What it does |
|---|---|---|
| `gitnexus_context` | GitNexus (PolyForm-Noncommercial) | Structural graph context for a symbol |
| `gitnexus_impact` | GitNexus | Blast-radius impact via graph (auto-indexes) |
| `gitnexus_ai_context` | GitNexus | Task-scoped, graph-aware context bundle |
| `markitdown_convert` | MarkItDown (MIT) | Convert PDF/DOCX/PPTX/XLSX/image → Markdown |
| `codocs_check` | codocs (built-in) | Find broken co-located `.doc.md` references |

**Total: 34 tools** = 5 core + 2 VFS + 2 AST + 1 arch + 2 runtime + 2 worktree + 10 subsystem + 5 memory + 5 external

---

## 🔌 Bridged External Tools (Never Vendored)

To keep the bundle MIT-licensed and lightweight, these upstream tools are
**bridged** (installed and updated independently, never vendored):

| Tool | Purpose | License | Setup |
|---|---|---|---|
| **[GitNexus](https://github.com/abhigyanpatwari/GitNexus)** | Upstream code knowledge graph | PolyForm-Noncommercial | `sin gitnexus setup` |
| **[Simone-MCP](https://github.com/OpenSIN-Code/Simone)** | Advanced code intelligence + LSP | Varies | Auto-detected during `sin bootstrap` |
| **[MarkItDown](https://github.com/microsoft/markitdown)** | Document → Markdown converter | MIT | `sin markitdown setup` |
| **[RTK](https://github.com/rtk-ai/rtk)** | Token-saving shell proxy (60-90% reduction) | Apache-2.0 | `sin rtk setup` |

---

## 🛡️ Graceful Degradation

Every subsystem is an **optional** dependency. If a subsystem is missing, the
MCP server detects it and gracefully falls back. The bundle never crashes on
a missing optional dep.

```bash
pip install -e ".[all]"     # All 8 Python subsystems + sin-brain + LSP
pip install -e ".[memory]"  # Just sin-brain (memory tools)
pip install -e ".[lsp]"     # tree-sitter + 4 parsers (Python <3.14 only)
pip install -e "."          # Minimal — uses graceful fallbacks
```

Check what's installed:
```bash
sin status
```

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ sin-serve MCP server (stdio)                                │
│   34 tools exposed (5 core + 2 VFS + 2 AST + 1 arch +       │
│   2 runtime + 2 worktree + 10 subsystem + 5 memory +        │
│   5 external bridges)                                       │
└─────────────────────────────────────────────────────────────┘
                          │
       ┌──────────────────┼──────────────────┐
       │                  │                  │
       ▼                  ▼                  ▼
┌────────────┐   ┌──────────────┐   ┌──────────────────┐
│ 7 Go tools │   │ 8 Python     │   │ 5 External       │
│ (grasp,    │   │ subsystems   │   │ bridges          │
│  scout,    │   │ (sckg, ibd,  │   │ (GitNexus,       │
│  discover, │   │  poc, efsm,  │   │  Simone-MCP,     │
│  execute,  │   │  adw, oracle,│   │  MarkItDown,     │
│  map,      │   │  orchestration│  │  RTK, codocs)    │
│  harvest,  │   │  review-     │   │                  │
│  orchestrate)│ │  interface)  │   │                  │
└────────────┘   └──────────────┘   └──────────────────┘
                          │
                          ▼
                  ┌──────────────┐
                  │ sin-brain    │
                  │ (SQLite+FTS5 │
                  │ memory)      │
                  └──────────────┘
```

---

## 🧰 CLI Commands

| Command | Description |
|---|---|
| `sin status` | Show which subsystems are installed/available |
| `sin bootstrap [repo]` | Initialize available subsystems (graph, baselines, ledger) |
| `sin review <a> <b>` | Semantic review of a change (IBD) |
| `sin verify <module> <fn>` | Proof-of-correctness for a function (POC) |
| `sin debt [root]` | Architectural debt overview (ADW) |
| `sin preflight [root]` | Ensure GitNexus graph context is fresh before agents code |
| `sin gitnexus setup` | Wire GitNexus MCP into OpenCode / Codex / Hermes |
| `sin gitnexus index\|status\|doctor\|context\|impact\|ai-context` | GitNexus graph operations |
| `sin markitdown setup\|doctor\|convert` | Wire/convert via MarkItDown (doc → Markdown) |
| `sin rtk setup\|doctor\|gain` | Wire RTK token-saving proxy into agents |
| `sin codocs check [root]` | Validate co-located `.doc.md` references (built-in) |
| `sin codocs list [root]` | List all CoDocs references and whether they resolve |
| `sin codocs install-skill` | Install the CoDocs agent skill (Hermes / OpenCode) |
| `sin sin-code run <tool> [args]` | Run a SIN-Code Go tool (discover, execute, map, grasp, scout, harvest, orchestrate) |
| `sin sin-code agents-md` | Generate AGENTS.md with SIN-Code Tool Suite rules |
| `sin serve` | Unified MCP server across available subsystems |

---

## 🦫 SIN-Code Go Tools (v2)

The next-generation SIN-Code tools are Go binaries that replace OpenCode's
built-in tools (auto-installed by `install.sh`):

| Tool | Purpose | Version |
|---|---|---|
| `discover` | File discovery with pattern matching, relevance scoring, dependency mapping | v0.2.5 |
| `execute` | Safe command execution with secret redaction, timeout, error analysis | v0.2.4 |
| `map` | Architecture analysis with module mapping, entry points, hot paths | v0.2.5 |
| `grasp` | Single-file deep analysis with structure, dependencies, context | v0.2.4 |
| `scout` | Code search with regex, semantic, symbol, usage modes | v0.1.5 |
| `harvest` | URL/API fetching with caching, structure extraction, auth management | v0.1.4 |
| `orchestrate` | Task management with dependencies, parallel execution, rollback | v0.1.6 |

Install manually:
```bash
go install github.com/OpenSIN-Code/SIN-Code-Discover-Tool/cmd/discover@latest
go install github.com/OpenSIN-Code/SIN-Code-Execute-Tool/cmd/execute@latest
go install github.com/OpenSIN-Code/SIN-Code-Map-Tool/cmd/map@latest
go install github.com/OpenSIN-Code/SIN-Code-Grasp-Tool/cmd/grasp@latest
go install github.com/OpenSIN-Code/SIN-Code-Scout-Tool/cmd/scout@latest
go install github.com/OpenSIN-Code/SIN-Code-Harvest-Tool/cmd/harvest@latest
go install github.com/OpenSIN-Code/SIN-Code-Orchestrate-Tool/cmd/orchestrate@latest
```

---

## 📚 Documentation

- [INSTALL.md](./INSTALL.md) — Detailed installation and troubleshooting
- [CHANGELOG.md](./CHANGELOG.md) — Version history
- [docs/USAGE.md](./docs/USAGE.md) — Deep dive into CLI + MCP tool usage
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md) — Configuration reference
- [docs/EXTERNAL_TOOLS.md](./docs/EXTERNAL_TOOLS.md) — Full compatibility matrix
- [docs/GITNEXUS.md](./docs/GITNEXUS.md) — How to wire the mandatory graph context
- [docs/CODOCS.md](./docs/CODOCS.md) — Co-located `.doc.md` standard
- [docs/adr/](./docs/adr/) — Architecture Decision Records
- [CONTRIBUTING.md](./CONTRIBUTING.md) — How to contribute

---

## 🤝 License

MIT — see [LICENSE](./LICENSE).

Bridged tools retain their original licenses:
- GitNexus: PolyForm-Noncommercial
- MarkItDown: MIT
- RTK: Apache-2.0

Part of the [SIN-Code](https://github.com/OpenSIN-Code) agent-engineering stack.
