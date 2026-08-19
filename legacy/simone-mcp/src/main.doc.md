# `src/main.py` — ASGI Application Factory

What this file does: builds the FastAPI `app` for ASGI servers (uvicorn, gunicorn). The `app` instance is what `uvicorn src.main:app` exposes.

## Dependency map

- Imports: `simone_mcp.http_app.create_app`.
- Imported by: ASGI runners.

## Endpoints exposed by `app`

| Path                                | Purpose                                   |
|-------------------------------------|-------------------------------------------|
| `/mcp`                              | MCP streamable HTTP transport              |
| `/a2a/v1`                           | A2A JSON-RPC                               |
| `/health`                           | Readiness probe                            |
| `/dashboard`                        | HTML status page                           |
| `/.well-known/agent.json`           | A2A agent card                             |
| `/.well-known/oauth-client.json`    | OAuth 2.1 client metadata                  |
| `/.well-known/oauth-authorization-server` | OAuth 2.1 AS metadata                |

## Usage

```bash
uvicorn src.main:app --host 0.0.0.0 --port 8234
```

## Caveats / footguns

- `app` is built at import time. Tests should use `create_app()` directly for isolation.
