# SIN-Code Bundle

> One CLI and one MCP server that orchestrate the entire SIN-Code
> agent-engineering stack.

[![Python](https://img.shields.io/badge/python-3.11%2B-blue)](https://www.python.org/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

Part of the [SIN-Code](https://github.com/OpenSIN-Code) agent-engineering stack.

## What is SIN-Code?

A set of state-of-the-art tools that give AI coding agents the signals they
actually lack — structural knowledge, semantic diffs, correctness proofs,
ephemeral test environments, debt/cost guardrails, and an independent
verification oracle.

| Repo | Role |
|------|------|
| [SCKG](https://github.com/OpenSIN-Code/SIN-Code-Semantic-Codebase-Knowledge-Graphs) | Semantic codebase knowledge graph |
| [IBD](https://github.com/OpenSIN-Code/SIN-Code-Intent-Based-Diffing) | Intent-based semantic diffing |
| [POC](https://github.com/OpenSIN-Code/SIN-Code-Proof-of-Correctness) | Lightweight proof of correctness |
| [EFSM](https://github.com/OpenSIN-Code/SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration) | Ephemeral full-stack mocking |
| [ADW](https://github.com/OpenSIN-Code/SIN-Code-Architectural-Debt-Watchdogs) | Architectural debt & cost watchdog |
| [Oracle](https://github.com/OpenSIN-Code/SIN-Code-Verification-Oracle) | Independent verification oracle |
| [Discover](https://github.com/OpenSIN-Code/SIN-Code-Discover-Tool) | File discovery with pattern matching, relevance scoring, dependency mapping |
| [Execute](https://github.com/OpenSIN-Code/SIN-Code-Execute-Tool) | Safe command execution with secret redaction, timeout, error analysis |
| [Map](https://github.com/OpenSIN-Code/SIN-Code-Map-Tool) | Architecture analysis with module mapping, entry points, hot paths |
| [Grasp](https://github.com/OpenSIN-Code/SIN-Code-Grasp-Tool) | Single-file deep analysis with structure, dependencies, context |
| [Scout](https://github.com/OpenSIN-Code/SIN-Code-Scout-Tool) | Code search with regex, semantic, symbol, usage modes |
| [Harvest](https://github.com/OpenSIN-Code/SIN-Code-Harvest-Tool) | URL/API fetching with caching, structure extraction, auth management |
| [Orchestrate](https://github.com/OpenSIN-Code/SIN-Code-Orchestrate-Tool) | Task management with dependencies, parallel execution, rollback |
| [GitNexus](https://github.com/abhigyanpatwari/GitNexus) | Upstream code knowledge graph — bridged, mandatory graph context for agents |
| [MarkItDown](https://github.com/microsoft/markitdown) | Upstream doc→Markdown converter — bridged, document context for agents |
| [RTK](https://github.com/rtk-ai/rtk) | Upstream token-saving command proxy — bridged, 60-90% fewer command tokens |
| CoDocs | Co-located docs standard (`.doc.md` companions) — built into the bundle |

## What the Bundle does

- Provides a single `sin` CLI over all subsystems.
- Exposes a unified MCP server (`sin serve` or `sin-serve`) so one entry
  serves agents **28 tools** out of the box.
- **Auto-detects and installs external MCP servers**: `gitnexus` (graph context)
  and `simone-mcp` (code intelligence) are checked and installed if missing.
- **Checks 8 Python subsystems**: SCKG, IBD, POC, EFSM, ADW, Oracle,
  Orchestration, Review-Interface — reports what's installed and suggests
  `pip install -e` commands for missing ones.
- **Degrades gracefully**: each subsystem is an optional dependency. The bundle
  detects which are installed (`sin status`) and only wires up what's available.

## MCP tools exposed (34 total)

The `sin-serve` MCP server (or equivalently `sin serve`) replaces opencode's
native read/write/edit/bash/search with SIN-Code tools that add structural
understanding, secret-redaction, hashline-anchored edits, and semantic
URI resolution.

| # | Tool | Replaces | Subsystem | What it does | **Why better than native / other tools** |
|---|------|----------|-----------|--------------|------------------------------------------|
| 1 | **`sin_read`** | `read` | vfs + grasp | URI-scheme (`sckg://`, `poc://`, `ibd://`, etc.) aware, size-safe, summarize mode | **Native `read` dumps 10MB+ into context** → blows LLM budget. `sin_read` returns size-aware summary or truncates at `max_chars`. URI schemes give **semantic context** (e.g. `sckg://module/auth/dependencies` returns the dependency graph, not 2000 lines of unrelated code). |
| 2 | **`sin_write`** | `write` | atomic + AST | Atomic write with auto-backup + syntax pre-validation for .py/.ts/.js/.go | **Native `write` creates half-written files on crash**. `sin_write` writes to a tempfile, then `replace()` — atomic on POSIX. **Compiles .py before writing** so a syntax error never reaches disk (auto-restores from backup). Prevents the classic "agent wrote broken Python and now nothing imports" loop. |
| 3 | **`sin_edit`** | `edit` | hashline | Hashline-anchored semantic patches (line-shift resilient, content-hash) | **Native `edit` uses line numbers** → breaks when the file is reformatted, the user adds a line above, or two edits race. `sin_edit` anchors by **content-hash** of the surrounding context — survives reformat, line shifts, and concurrent edits. The agent's edits don't go stale silently. |
| 4 | **`sin_bash`** | `bash` | execute (Go) | Safe exec with secret-redaction + timeout + structured JSON (safety_check, retry_info, learned_patterns) | **Native `bash` runs raw shell and leaks secrets in output** (PATs in env, tokens in error messages). `sin_bash` wraps `execute` (Go binary) which redacts tokens/keys automatically, enforces timeout, and returns structured JSON with `safety_check.is_safe` + `retry_info.attempt` + `learned_patterns` (so the agent learns which commands work). |
| 5 | **`sin_search`** | `search`/`find`/`grep`/`glob` | scout (Go) + Python | Semantic + regex + symbol + usage search, single-file + directory | **Native `search`/`grep`/`find` are 4 separate tools** doing overlapping things. `sin_search` unifies all 4 with one tool: regex, semantic, symbol, usage. **Searches inside functions / class bodies** (semantic mode) — not just text matches. Falls back to Python-regex when scout binary is missing, so it always works. |
| 6 | `sin_vfs_resolve` | (new) | vfs | Resolve URI scheme → structured content | **No native equivalent**. Lets the agent query **scoped semantic data**: `sckg://module/auth/dependencies` returns just the auth module's deps, not the whole repo. Massive context-window savings. |
| 7 | `sin_vfs_schemes` | (new) | vfs | List all available URI schemes | **No native equivalent**. Agent can introspect what's queryable. Used as a discovery helper before calling `sin_vfs_resolve`. |
| 8 | `sin_ast_edit` | (new) | ast_edit | Tree-sitter AST-based edit with POC verification | **Beyond `sin_edit`**: this understands **AST structure** (Python/JS/TS/Go), not just text. Refactors a function by name without touching its body, renames a class across the file, etc. POC verifies the result is still correct. Falls back to hashline if tree-sitter is missing. |
| 9 | `sin_hashline_validate` | (new) | hashline | Validate a previously-created hashline patch can still be applied | **No native equivalent**. Before applying a stored patch, check if the file has drifted. Avoids the silent-failure mode where a patch is "applied" but does nothing because the content changed. |
| 10 | `impact` | (new) | sin-code-sckg | Blast-radius impact analysis for a symbol | **No native equivalent**. Native tools can't answer "if I change `processPayment()`, what breaks?". `impact` returns the full downstream call graph — files, functions, tests. |
| 11 | `semantic_diff` | (new) | sin-code-ibd | Semantic intent diff between two files | **Beyond `git diff`**: `git diff` shows line-level changes. `semantic_diff` shows **intent** ("auth flow was refactored", "caching was added") and **risk score**. Agent knows whether a 5-line change is cosmetic or a 2-week architectural shift. |
| 12 | `semantic_review` | (new) | sin-code-ibd | Intent + risk in one call | Same as `semantic_diff` + verdict in one call. |
| 13 | `architectural_debt` | (new) | sin-code-adw | Current architectural debt score | **No native equivalent**. Quantifies technical debt (god modules, circular imports, hot paths without tests) on a 0-100 scale. Agent can prioritize refactors by ROI. |
| 14 | `verify_tests` | (new) | sin-code-oracle | Verify agent-generated code (security/perf/correctness) | **No native equivalent**. Independent verification — checks for OWASP Top 10, CWE Top 25, performance pitfalls. Agent gets a second opinion on its own code before the human reviewer sees it. |
| 15 | `prove` | (new) | sin-code-poc | Generate and verify proofs of correctness | **No native equivalent**. Lightweight formal proofs (Hoare-style pre/post-conditions) on agent-generated functions. Catches off-by-one, wrong null-handling, edge cases the LLM missed. |
| 16 | `mock_env` | (new) | sin-code-efsm | Manage ephemeral full-stack mock environment | **No native equivalent**. Spin up a full-stack mock (Postgres + Redis + API server) in 10s, run integration tests, tear it down. No docker-compose yak-shaving. |
| 17 | `orchestrate` | (new) | sin-code-orchestration | Submit a task to the multi-agent orchestrator | **Beyond native `task`**: dependency-graph-aware multi-agent. Submit 10 tasks, orchestrator runs them in topological order with parallel execution where possible, rollback on failure. |
| 18 | `task_status` | (new) | sin-code-orchestration | Get status of an orchestrated task | Polling endpoint for in-flight orchestrated tasks. |
| 19 | `review` | (new) | sin-code-review-interface | SOTA review on a single file | **Beyond `git diff` + manual reading**: automated review combining diff + debt + tests + style in one structured output. Pre-PR quality gate. |
| 20 | `recall_tool` | (new) | sin-brain | Search memory tiers (recall/archival/graph) | **No native equivalent**. Cross-session, cross-project memory. The agent remembers yesterday's decisions, last week's debugging, last sprint's conventions. Persistent context. |
| 21 | `remember_tool` | (new) | sin-brain | Persist a memory entry (decision/convention/fix/pitfall/preference) | **No native equivalent**. Agent decides what's worth remembering and tags it (`decision` / `convention` / `fix` / `pitfall` / `preference`). |
| 22 | `forget_tool` | (new) | sin-brain | Delete a memory entry by id | GDPR / cleanup. |
| 23 | `pin_tool` | (new) | sin-brain | Pin a memory entry (never evicted) | High-importance facts (e.g. "this project uses Python 3.14") never get garbage-collected. |
| 24 | `link_evidence_tool` | (new) | sin-brain | Attach a subsystem verdict to a memory | Memory entries can be **backed by proof** (oracle verified, POC proven, IBD-reviewed). Agent doesn't have to re-litigate past decisions. |
| 25 | `gitnexus_context` | (new) | gitnexus | Structural graph context for a symbol | **Beyond `read` + manual analysis**: returns the full structural context — callers, callees, type info, tests. The agent understands the code, not just the bytes. |
| 26 | `gitnexus_impact` | (new) | gitnexus | Blast-radius impact via graph (auto-indexes) | Same as `impact` but uses gitnexus's pre-built graph (faster than SCKG re-index). |
| 27 | `gitnexus_ai_context` | (new) | gitnexus | Task-scoped, graph-aware context bundle | **Context on-demand**: agent says "I'm about to refactor auth" → returns the 50 lines of context that matter, not 50,000. Massive token savings. |
| 28 | `markitdown_convert` | (new) | markitdown | Convert PDF/DOCX/PPTX/XLSX/image → Markdown | **No native equivalent**. Agent can read PDFs, Word docs, Excel sheets, images. Useful for working with design docs, requirements, tickets. |
| 26a | `sin_runtime_trace` | (new) | dap_bridge | Start DAP debug session for a function (debugpy/dlv/node) |
| 26b | `sin_stop_trace` | (new) | dap_bridge | Stop an active DAP session |
| 26c | `sin_check_architecture` | (new) | interceptor | Pre-flight: validate tool call against architectural rules |
| 26d | `sin_create_worktree` | (new) | orchestration_worktrees | Create isolated git worktree for parallel agent tasks |
| 26e | `sin_cleanup_worktree` | (new) | orchestration_worktrees | Clean up worktree (optionally merge back) |
| 29 | `codocs_check` | (new) | codocs | Find broken co-located `.doc.md` references | **No native equivalent**. Enforces the CoDocs standard: every code file has a `.doc.md` companion. CI integration via ceo-audit. |

### Native → SIN tool coverage (mandatory in `~/.config/opencode/opencode.json`)

| Native opencode tool | SIN replacement | Notes |
|----------------------|-----------------|-------|
| `read` | `sin_read` | URI-aware, size-safe, summarize mode |
| `write` | `sin_write` | Atomic + syntax-validate + auto-backup |
| `edit` | `sin_edit` | Hashline-anchored, line-shift resilient |
| `bash` | `sin_bash` | Secret-redaction + timeout + structured JSON |
| `search` | `sin_search` | Semantic + regex + symbol + usage |
| `find` | `sin_search` (path=dir) | Directory rglob fallback |
| `grep` | `sin_search` (search_type=regex) | Pattern search across files |
| `glob` | `sin_search` (regex) | Pattern-match file paths |
| `list` | `sin_read` (path=dir) | Returns `{"type": "directory", "items": [...]}` |
| `webfetch` | `sin_bash` (`curl ...`) or `sin_search` (URL) | HTTP fetch via bash |
| `task` | `sin_bash` (`sin orchestrate ...`) | Subagent via orchestrate subsystem |

The `~/.config/opencode/opencode.json` shipped with this repo sets:
```json
{
  "tools": { "read/write/edit/bash/search/find/grep/glob/list/webfetch/task": false },
  "mcp": { "sin-code-bundle": { "type": "local", "command": ["sin", "serve"], "enabled": true } }
}
```
Native tools are disabled by the global AGENTS.md mandate so agents use
the SIN-Code replacements exclusively.

## Quickstart

### One-command full install

```bash
# Bootstraps the entire SIN-Code stack (7 Go tools + Python bundle + MCP config + externals)
bash install.sh
```

This installs all 7 Go tools, the Python bundle in editable mode, auto-detects
and installs **gitnexus** (graph context) and **simone-mcp** (code intelligence),
checks for **SIN-Brain** (docs-only), verifies all 8 Python subsystems, and
registers everything in `~/.config/opencode/opencode.json`.

**Flags:** `--help` `--dry-run` `--verbose` `--force` `--skip-go` `--skip-external`

**Environment overrides:**
```bash
SIN_CODE_BIN_DIR=~/custom-bin SIN_CODE_REPOS_DIR=~/my-repos bash install.sh
```

**Full installation takes ~2–5 minutes** (depending on Go build cache and
whether npm packages need downloading). Re-runs are safe and idempotent.

See `install.sh --help` for full details. The companion docs are at
[`install.sh.doc.md`](./install.sh.doc.md).

### Manual install (step by step)

```bash
# Install the subsystems you want, then the bundle:
pip install -e ../SIN-Code-Semantic-Codebase-Knowledge-Graphs
pip install -e ../SIN-Code-Intent-Based-Diffing
pip install -e ../SIN-Code-Proof-of-Correctness
pip install -e ../SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration
pip install -e ../SIN-Code-Architectural-Debt-Watchdogs
pip install -e ../SIN-Code-Verification-Oracle
pip install -e ../SIN-Code-Orchestration
pip install -e ../SIN-Code-Review-Interface
pip install -e .

sin status # show which subsystems are available
sin bootstrap . # initialize available subsystems for a repo
sin serve # unified MCP server
```

## Commands

| Command | Description |
|---------|-------------|
| `sin status` | Show which subsystems are installed/available. |
| `sin bootstrap [repo]` | Initialize available subsystems (graph, baselines, ledger). |
| `sin review <a> <b>` | Semantic review of a change (IBD). |
| `sin verify <module> <fn>` | Proof-of-correctness for a function (POC). |
| `sin debt [root]` | Architectural debt overview (ADW). |
| `sin preflight [root]` | Ensure GitNexus graph context is fresh before agents code. |
| `sin gitnexus setup` | Wire GitNexus MCP into OpenCode / Codex / Hermes. |
| `sin gitnexus index\|status\|doctor\|context\|impact\|ai-context` | GitNexus graph operations. |
| `sin markitdown setup\|doctor\|convert` | Wire/convert via MarkItDown (doc→Markdown). |
| `sin rtk setup\|doctor\|gain` | Wire RTK token-saving proxy into agents. |
| `sin codocs check [root]` | Validate co-located `.doc.md` references (built-in). |
| `sin codocs list [root]` | List all CoDocs references and whether they resolve. |
| `sin codocs install-skill` | Install the CoDocs agent skill (Hermes / OpenCode). |
| `sin sin-code run <tool> [args]` | Run a SIN-Code Go tool (discover, execute, map, grasp, scout, harvest, orchestrate). |
| `sin sin-code agents-md` | Generate AGENTS.md with SIN-Code Tool Suite rules. |
| `sin serve` | Unified MCP server across available subsystems. |

## GitNexus: mandatory graph context

Coder agents should never edit a repo "blind". The bundle bridges
[GitNexus](https://github.com/abhigyanpatwari/GitNexus) (kept as the upstream
original, **not** vendored — it is PolyForm-Noncommercial while the bundle is
MIT) and makes its code knowledge graph available to every agent:

```bash
sin gitnexus setup   # wire OpenCode + Codex + Hermes to the GitNexus MCP server
sin preflight        # auto-build/refresh the graph before any agent task
```

Requires Node.js >= 18 (`npx`). See [docs/GITNEXUS.md](./docs/GITNEXUS.md).

## More external tools: MarkItDown & RTK

Two more best-in-class upstream tools are bridged the same way (installed and
updated independently, never vendored), so every coder agent gets them:

```bash
sin markitdown setup   # MarkItDown MCP: convert PDF/Office/images to Markdown
sin rtk setup          # RTK: route agent shell commands through a token-saving proxy
```

- **[MarkItDown](https://github.com/microsoft/markitdown)** (MIT) — document
  context. Install: `pip install markitdown-mcp "markitdown[all]"`.
- **[RTK](https://github.com/rtk-ai/rtk)** (Apache-2.0) — 60-90% fewer tokens on
  common commands. Install: `brew install rtk`.

See [docs/EXTERNAL_TOOLS.md](./docs/EXTERNAL_TOOLS.md) for the full matrix.

## SIN-Code Go Tools (v2)

The next-generation SIN-Code tools are Go binaries that replace OpenCode's built-in tools:

```bash
# Install all 7 tools
go install github.com/OpenSIN-Code/SIN-Code-Discover-Tool/cmd/discover@latest
go install github.com/OpenSIN-Code/SIN-Code-Execute-Tool/cmd/execute@latest
go install github.com/OpenSIN-Code/SIN-Code-Map-Tool/cmd/map@latest
go install github.com/OpenSIN-Code/SIN-Code-Grasp-Tool/cmd/grasp@latest
go install github.com/OpenSIN-Code/SIN-Code-Scout-Tool/cmd/scout@latest
go install github.com/OpenSIN-Code/SIN-Code-Harvest-Tool/cmd/harvest@latest
go install github.com/OpenSIN-Code/SIN-Code-Orchestrate-Tool/cmd/orchestrate@latest

# Or via the bundle
sin sin-code run discover --help
sin sin-code run execute --help
sin sin-code run map --help
sin sin-code run grasp --help
sin sin-code run scout --help
sin sin-code run harvest --help
sin sin-code run orchestrate --help

# Generate AGENTS.md for any repo
sin sin-code agents-md --output AGENTS.md
```

| Tool | Purpose | Status |
|------|---------|--------|
| discover | File discovery with pattern matching, relevance scoring, dependency mapping | v0.2.5 ✅ |
| execute | Safe command execution with secret redaction, timeout, error analysis | v0.2.4 ✅ |
| map | Architecture analysis with module mapping, entry points, hot paths | v0.2.5 ✅ |
| grasp | Single-file deep analysis with structure, dependencies, context | v0.2.4 ✅ |
| scout | Code search with regex, semantic, symbol, usage modes | v0.1.5 ✅ |
| harvest | URL/API fetching with caching, structure extraction, auth management | v0.1.4 ✅ |
| orchestrate | Task management with dependencies, parallel execution, rollback | v0.1.6 ✅ |

## Documentation

- [INSTALL.md](./INSTALL.md)
- [docs/USAGE.md](./docs/USAGE.md)
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md)
- [docs/GITNEXUS.md](./docs/GITNEXUS.md)
- [docs/EXTERNAL_TOOLS.md](./docs/EXTERNAL_TOOLS.md)
- [docs/CODOCS.md](./docs/CODOCS.md)
- [docs/adr](./docs/adr/) — Architecture Decision Records
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [CHANGELOG.md](./CHANGELOG.md)

## License

MIT — see [LICENSE](./LICENSE).
# Test change
