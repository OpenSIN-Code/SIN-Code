# Simone MCP

- Team: Team - Coding
- Team Manager: SIN-Coding-CEO
- Slug: simone-mcp
- Purpose: Ultra-Duo code worker with Serena-grade LSP semantics and A2A-native async runtime.

## Commands

- `activate_simone`
- `activate_simone serve-mcp`
- `activate_simone print-card`
- `activate_simone run-action '{"action":"agent.help"}'`

## Runtime

- FastAPI server with `/health`, `/dashboard`, `/.well-known/agent-card.json`, `/a2a/v1`
- Supabase Realtime listener for `INSERT` events on `public.a2a_tasks`
- Cloud semantic memory via PostgreSQL/pgvector
