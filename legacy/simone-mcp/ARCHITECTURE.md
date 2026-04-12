# Simone MCP - Ultra-Duo Team Coder Agent

**Status:** 🟢 Live runtime + ongoing hardening  
**A2A Team:** `team-coding`  
**Runtime Target:** Hugging Face VM (Stateless)  
**Event Bus:** Supabase Realtime  

## Overview

Simone MCP is a specialized **LSP-powered code agent** that integrates Serena MCP's semantic analysis capabilities with 2026 A2A best practices. Unlike Serena, Simone is:

- ✅ **A2A-Native**: Full `agent-card.json` discovery
- ✅ **Async-First**: Event-driven via Supabase Realtime
- ✅ **Cloud Memory**: PostgreSQL/pgvector semantic memory
- ✅ **Self-Healing**: Keep-alive + auto-restart on HF VM

## Architecture

```
┌─────────────────────────────────┐
│ Orchestrator (e.g. A2A-Coding-CEO) │
│ - Plans architecture            │
│ - Delegates via A2A             │
└──────────────┬──────────────────┘
               │ A2A Protocol
               ▼
┌─────────────────────────────────┐
│ Simone MCP (Worker)             │
│ - LSP symbol operations         │
│ - Stateless HF VM runtime       │
│ - pgvector memory               │
└─────────────────────────────────┘
```

## A2A Surface

### Endpoints
- `GET /.well-known/agent-card.json` - A2A discovery
- `GET /health` - Health check
- `POST /a2a/v1` - A2A message endpoint

### Tools (A2A Actions)
| Action | Description |
|--------|-------------|
| `code.find_symbol` | Find symbol by name (LSP) |
| `code.find_references` | Find all references to symbol |
| `code.insert_after_symbol` | Insert code after symbol |
| `code.replace_symbol_body` | Replace symbol body |
| `code.get_project_overview` | Get project structure |
| `code.semantic_search` | Vector search in pgvector memory |

## Deployment

### Hugging Face Space
- **Space ID:** `delqhi/simone-mcp`
- **Runtime:** Python 3.11 + FastAPI
- **Keep-Alive:** Ping every 20min

### Public Surfaces
- **Landing page:** `https://a2a.delqhi.com/agents/simone-mcp`
- **HF runtime:** `https://delqhi-simone-mcp.hf.space`

### Environment Variables
```bash
SUPABASE_URL=required
SUPABASE_SERVICE_ROLE_KEY=required
JETSTREAM_URL=required
SIMONE_LSP_LANGUAGES=python,typescript,javascript
```

## Development Status

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1: Foundation | 🟡 In Progress | Basic MCP server, LSP integration |
| Phase 2: A2A Integration | ⚪ Pending | Supabase events, pgvector memory |
| Phase 3: Production | ⚪ Pending | Security, multi-language, docs |

## Related Issues
- Issue #311: Create Simone MCP
- Issue #312: Serena MCP gap analysis
