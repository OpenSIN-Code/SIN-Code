# A2A Card: Simone MCP

**Agent Type:** Code Worker  
**Team:** `team-coding`  
**Maturity:** 🟡 Beta baseline

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

## Capabilities

| Capability | Type | Status |
|------------|------|--------|
| `code.find_symbol` | Tool | ✅ implemented |
| `code.find_references` | Tool | ✅ implemented |
| `code.replace_symbol_body` | Tool | ✅ implemented |
| `code.insert_after_symbol` | Tool | ✅ implemented |
| `code.project_overview` | Tool | ✅ implemented |
| `memory.query` | Tool | ✅ facade implemented |
| `simone.mcp.health` | Tool | ✅ implemented |

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