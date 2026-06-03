# `http_app.py` — FastAPI Application (HTTP / SSE / A2A)

What this file does: the HTTP front door. Wires CORS, origin validation, OAuth 2.1 bearer auth, per-IP rate limiting, body size cap, the `/mcp` (POST/GET/DELETE) endpoint, `/a2a/v1`, and the `/.well-known/` discovery endpoints.

## Dependency map

- Imports: `fastapi`, `jwt` (PyJWT), `starlette.middleware.cors`, internal: `a2a_handler`, `correlation`, `core` (agent card, dashboard, MCP endpoint constants), `hybrid_memory.shutdown_stores`, `protocol` (handle_mcp_request, SSE constants).
- Imported by: `src/main.py` (builds `app` from `create_app()`).

## Public API

| Function                  | Purpose                                                          |
|---------------------------|------------------------------------------------------------------|
| `create_app()`            | Build and configure the FastAPI app                              |
| `shutdown_stores`         | Close backend connections (called from FastAPI `lifespan`)        |

`create_app()` is the standard entry point; `src/main.py` uses it to expose `app`.

## Important config / limits

- **Rate limit: 100 req / 60s per IP** (env: `SIMONE_RATE_LIMIT_WINDOW`, `SIMONE_RATE_LIMIT_MAX`).
- **Body cap: 1 MiB** (env: `SIMONE_MAX_REQUEST_BODY`).
- **CORS allow-list** (env: `SIMONE_ALLOWED_ORIGINS`, comma-separated; default `localhost`, `127.0.0.1`, `opensin.ai`).
- **Auth: optional** (env: `SIMONE_AUTH_REQUIRED` = `true` / `1` enables). When enabled, requires a bearer token validated against `SIMONE_OAUTH_JWKS_URL`.
- **`Mcp-Session-Id` / `Mcp-Method` / `Mcp-Name` / `Mcp-Param-*` headers** are validated against the JSON-RPC body in MCP POSTs (HeaderMismatch → error code -32001).

## Design decisions

- **Why per-IP rate limiting, not per-token?** Per-token is correct but needs a token store; per-IP is good enough for typical deployments and trivially DoS-resistant.
- **Why fetch JWKS on every request?** Correctness over performance — a key rotation would otherwise require a restart. The HTTPS round-trip is the price.
- **Why `lifespan` instead of `@app.on_event("startup")`?** The latter is deprecated in modern FastAPI. `lifespan` is the recommended pattern.

## Usage example

```python
from simone_mcp.http_app import create_app

app = create_app()
# uvicorn my_module:app
```

## Caveats / footguns

- **CORS is enforced by origin string match** (not pattern). Wildcards are not supported.
- **The OAuth 2.1 verifier requires `pyjwt[crypto]`** for RS256/ES256. ES-only installations need `SIMONE_OAUTH_ALGORITHMS=ES256`.
- **Notifications are NOT pushed inline.** Streamable HTTP clients must use SSE or poll `tasks/get` to see async progress (SEP-2663).
- **Origin validation is bypassed** when no `Origin` header is set (server-to-server callers). Add a reverse proxy with auth if you need stronger guarantees.
