# A2A Card: Simone MCP

**Agent Type:** Specialized LSP Code Worker  
**Team:** `team-coding`  
**Maturity:** 🟡 Alpha  

## 🚀 Warum Simone MCP? (Business Case)

Simone MCP löst die kritischen Lücken von Serena MCP für den Enterprise-Einsatz 2026:

| Feature | Serena MCP | **Simone MCP** |
|---------|------------|----------------|
| **A2A Discovery** | ❌ Nein | ✅ Ja (`agent-card.json`) |
| **Memory** | 📁 Local File | ☁️ PostgreSQL/pgvector |
| **Communication** | 🔄 Sync (blocking) | ⚡ Async (event-driven) |
| **Dashboard** | 📟 Basic CLI | 🖥️ Enterprise UI |
| **Deployment** | 📦 Manual | 🚀 Self-healing HF VM |

## Purpose

Simone MCP bietet **symbol-level code analysis and editing** für die A2A-Flotte mittels LSP (Language Server Protocol). Es wird für komplexes Refactoring, Code-Navigation und strukturelle Code-Modifikationen delegiert.

## Capabilities

| Capability | Type | Status |
|------------|------|--------|
| `code.find_symbol` | Tool | 🟡 Planned |
| `code.find_references` | Tool | 🟡 Planned |
| `code.insert_after_symbol` | Tool | 🟡 Planned |
| `code.replace_symbol_body` | Tool | 🟡 Planned |
| `code.get_project_overview` | Tool | 🟡 Planned |
| `code.semantic_search` | Tool | 🟡 Planned |

## Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/.well-known/agent-card.json` | GET | A2A discovery |
| `/health` | GET | Health check |
| `/a2a/v1` | POST | A2A message handling |
| `/dashboard` | GET | Enterprise UI (`activate_simone`) |

## Runtime

- **Target:** Hugging Face Space (`delqhi/simone-mcp`)
- **Language:** Python 3.11+
- **Event Bus:** Supabase Realtime
- **Memory:** PostgreSQL/pgvector

## Dependencies

- Serena MCP `solid-lsp` library (LSP abstraction)
- Supabase (event bus + memory)
- FastAPI (MCP server)

## Owner

- **Team:** `team-coding`
- **On-Call:** @Delqhi

