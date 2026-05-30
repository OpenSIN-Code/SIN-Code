# Simone MCP

- Team: Team - Coding
- Team Manager: SIN-Coding-CEO
- Slug: simone-mcp
- Purpose: MCP 2026-06-30 production-grade code worker with symbol operations, Tasks v2 (SEP-2663), structured output, and A2A integration.
- MCP Protocol Version: 2026-06-30

## Quick Start (HOW TO RUN)

### Start HTTP Server
```bash
cd /Users/jeremy/dev/Simone-MCP
PYTHONPATH=src python3 src/cli.py serve
# Server runs on http://localhost:8234
# Endpoints: /mcp, /a2a/v1, /health, /dashboard, /.well-known/agent-card.json
```

### Start MCP stdio (for OpenCode/Codex)
```bash
cd /Users/jeremy/dev/Simone-MCP
PYTHONPATH=src python3 src/cli.py serve-mcp
```

### Test a Tool Directly
```bash
cd /Users/jeremy/dev/Simone-MCP
PYTHONPATH=src python3 src/cli.py run-action '{"action":"sin_simone_mcp_health"}'
PYTHONPATH=src python3 src/cli.py run-action '{"action":"sin_simone_mcp_symbol_search","query":"myFunction"}'
PYTHONPATH=src python3 src/cli.py tool-list
```

### Run Tests
```bash
cd /Users/jeremy/dev/Simone-MCP
PYTHONPATH=src pytest tests/ -v
```

## Commands

- `activate_simone`
- `activate_simone serve` — Start HTTP server (port 8234)
- `activate_simone serve-mcp` — Start MCP stdio server
- `activate_simone print-card` — Print agent discovery card
- `activate_simone run-action '{"action":"agent.help"}'`

## Runtime

- FastAPI server with `/health`, `/dashboard`, `/.well-known/agent-card.json`, `/a2a/v1`, `/mcp`
- Streamable HTTP + stdio MCP support (both delegate to protocol.py)
- Hybrid memory via Qdrant + Neo4j with optional Supabase event wiring

## Available MCP Tools

| Tool | Description | Task Support |
|:---|:---|:---|
| `sin_simone_mcp_health` | Server health check | forbidden |
| `sin_simone_mcp_symbol_search` | Find symbol definitions (LSP-powered) | forbidden |
| `sin_simone_mcp_structural_edit` | Replace/insert code via structural payload | forbidden |
| `sin_simone_mcp_memory_query` | Hybrid memory search (Qdrant + Neo4j) | forbidden |
| `sin_simone_mcp_find_references` | Find textual references to a symbol | forbidden |
| `sin_simone_mcp_project_overview` | Summarize workspace footprint | forbidden |

**All tools return `structuredContent` inline** (no task deferral). Call via `tools/call` and read `result.structuredContent`.

## MCP 2026-06-30 Compliance

- Tasks v2 (SEP-2663): `tasks/get` (inline result), `tasks/update`, `tasks/cancel` + `notifications/tasks`; server decides task creation autonomously; `resultType: "task"`; `io.modelcontextprotocol/tasks` extension
- HTTP Headers (SEP-2243): `Mcp-Method`, `Mcp-Name`, `Mcp-Param-*` validation with `-32001` HeaderMismatch
- List TTL (SEP-2549): `ttlMs` + `cacheScope` on all list responses
- Structured output: `structuredContent` + `outputSchema` (JSON Schema 2020-12) on all 6 tools
- Tool metadata: `title`, `execution.taskSupport` (forbidden)
- `resource_link` type in tool results
- Input validation errors → `isError: true` (SEP-1303, not JSON-RPC errors)
- `_meta` propagation on all response methods
- Version negotiation (highest <= client version)
- SSE `retry:` field, `MCP-Protocol-Version` HTTP header
- Session cleanup on `DELETE /mcp`

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **Simone-MCP** (1111 symbols, 1485 relationships, 41 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/Simone-MCP/context` | Codebase overview, check index freshness |
| `gitnexus://repo/Simone-MCP/clusters` | All functional areas |
| `gitnexus://repo/Simone-MCP/processes` | All execution flows |
| `gitnexus://repo/Simone-MCP/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

---

## 🧠 Simone MCP — Code Intelligence & Automation

Simone MCP bietet zusätzliche Code-Analyse-Tools via MCP:

**Verfügbare Tools:**
- `sin_simone_mcp_symbol_search` — Symbol-Suche im gesamten Workspace
- `sin_simone_mcp_find_references` — Alle Referenzen zu einem Symbol finden
- `sin_simone_mcp_project_overview` — Workspace-Footprint + Dateitypen
- `sin_simone_mcp_structural_edit` — Strukturelle Code-Edits (LSP-grade)
- `sin_simone_mcp_memory_query` — Cloud Semantic Memory (Kontext + Analysen)
- `sin_simone_mcp_health` — Server-Status und Capabilities

**IMMER verwenden für:**
- `sin_simone_mcp_symbol_search` statt grep für Symbol-Suche
- `sin_simone_mcp_find_references` vor Refactoring
- `sin_simone_mcp_project_overview` für schnellen Codebase-Überblick
- `sin_simone_mcp_structural_edit` für sichere, strukturierte Edits
