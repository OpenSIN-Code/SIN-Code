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
