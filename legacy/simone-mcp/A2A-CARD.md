# A2A Card: Simone MCP

**Agent Type:** Code Worker  
**Team:** `team-coding`  
**Maturity:** 🟢 Production  
**MCP Protocol:** 2026-06-30

## Purpose

Simone MCP is the OpenSIN code worker for symbol-level analysis, structural editing, MCP transport compatibility, and repo-aware execution.

## Why it exists

Simone MCP closes the gap between lightweight symbol tools and a production-facing MCP/A2A service:

| Surface | Old state | Current baseline |
|---------|-----------|------------------|
| Source of truth | JS stubs only | Real Python implementation |
| MCP | stdio-only template logic | stdio + streamable HTTP |
| Discovery | partial | `.well-known` metadata present |
| Auth posture | api key placeholder | OAuth 2.1-ready metadata and JWKS path |
| Memory | pgvector placeholder | Qdrant + Neo4j hybrid contract |
| Deployment | unclear | Docker + compose + HF-ready shape |

## Intelligence Core

| Model (Primary) | `openai/gpt-5.4` |
| Model (Fallback) | `nvidia/minimaxai/minimax-m2.7` |

## Capabilities

| Capability | Type | Task Support | Status |
|------------|------|-------------|--------|
| `sin_simone_mcp_health` | Tool | forbidden | ✅ implemented |
| `sin_simone_mcp_symbol_search` | Tool | forbidden | ✅ implemented |
| `sin_simone_mcp_find_references` | Tool | forbidden | ✅ implemented |
| `sin_simone_mcp_structural_edit` | Tool | forbidden | ✅ implemented |
| `sin_simone_mcp_memory_query` | Tool | forbidden | ✅ implemented |
| `sin_simone_mcp_project_overview` | Tool | forbidden | ✅ implemented |

### MCP 2026-06-30 Features

- Tasks v2 (SEP-2663): `tasks/get` (inline result), `tasks/update`, `tasks/cancel` + `notifications/tasks`; server decides task creation; `resultType: "task"`; `io.modelcontextprotocol/tasks` extension
- HTTP Headers (SEP-2243): `Mcp-Method`, `Mcp-Name`, `Mcp-Param-*` validation
- List TTL (SEP-2549): `ttlMs` + `cacheScope` on all list responses
- Structured output: `structuredContent` + `outputSchema` (JSON Schema 2020-12)
- Tool `title` and `execution.taskSupport` on all tools
- `resource_link` type in tool results
- Input validation as `isError: true` (SEP-1303)
- `_meta` propagation, version negotiation, SSE retry

## Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/.well-known/agent-card.json` | GET | A2A discovery |
| `/.well-known/agent.json` | GET | Agent metadata |
| `/.well-known/oauth-client.json` | GET | OAuth client metadata |
| `/.well-known/oauth-authorization-server` | GET | OAuth server metadata |
| `/health` | GET | Health probe |
| `/dashboard` | GET | Operator quick actions |
| `/a2a/v1` | POST | A2A JSON-RPC |
| `/mcp` | GET, POST, DELETE | MCP streamable HTTP |

## Runtime

- **Language:** Python 3.12+
- **Local MCP:** stdio loop
- **Remote MCP:** FastAPI streamable HTTP
- **Memory:** Qdrant + Neo4j
- **Optional event backplane:** Supabase
- **Default local port:** `8234`

## Deployment target

- local dev
- Docker runtime
- Hugging Face Space for compute and UI

## Owner

- **Team:** `team-coding`
- **On-Call:** `@Delqhi`