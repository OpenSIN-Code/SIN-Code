# Simone MCP

- Team: Team - Coding
- Team Manager: SIN-Coding-CEO
- Slug: simone-mcp
- Purpose: MCP 2026-06-30 production-grade code worker with symbol operations, Tasks v2 (SEP-2663), structured output, and A2A integration.
- MCP Protocol Version: 2026-06-30

## Commands

- `activate_simone`
- `activate_simone serve-mcp`
- `activate_simone print-card`
- `activate_simone run-action '{"action":"agent.help"}'`

## Runtime

- FastAPI server with `/health`, `/dashboard`, `/.well-known/agent-card.json`, `/a2a/v1`, `/mcp`
- Streamable HTTP + stdio MCP support (both delegate to protocol.py)
- Hybrid memory via Qdrant + Neo4j with optional Supabase event wiring

## MCP 2026-06-30 Compliance

- Tasks v2 (SEP-2663): `tasks/get` (inline result), `tasks/update`, `tasks/cancel` + `notifications/tasks`; server decides task creation autonomously; `resultType: "task"`; `io.modelcontextprotocol/tasks` extension
- HTTP Headers (SEP-2243): `Mcp-Method`, `Mcp-Name`, `Mcp-Param-*` validation with `-32001` HeaderMismatch
- List TTL (SEP-2549): `ttlMs` + `cacheScope` on all list responses
- Structured output: `structuredContent` + `outputSchema` (JSON Schema 2020-12) on all 6 tools
- Tool metadata: `title`, `execution.taskSupport` (forbidden/optional/required)
- `resource_link` type in tool results
- Input validation errors → `isError: true` (SEP-1303, not JSON-RPC errors)
- `_meta` propagation on all response methods
- Version negotiation (highest <= client version)
- SSE `retry:` field, `MCP-Protocol-Version` HTTP header
- Session cleanup on `DELETE /mcp`
