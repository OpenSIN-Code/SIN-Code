# `src/simone_mcp/http_app.py` — FastAPI HTTP Application

Partner file: `src/simone_mcp/http_app.py`

## Purpose
Production-grade FastAPI application with MCP 2.0 streamable HTTP + A2A + SSE support. Includes rate limiting, OAuth 2.1 JWT verification, CORS, origin validation, request body limits, and session management.

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `create_app()` | function | FastAPI factory with lifespan, middleware, and routes |
| `_mcp_post()` | async function | Handle POST /mcp (tools, resources, tasks, prompts) |
| `_mcp_get()` | async function | Handle GET /mcp (SSE event stream) |
| `_authorize_request()` | function | JWT Bearer token validation via JWKS |
| `_validate_origin()` | function | CORS origin whitelist check |
| `_check_rate_limit()` | function | Per-IP rate limiting (100 req/60s) |
| `_extract_client_ip()` | function | X-Forwarded-For aware IP extraction |
| `_verify_token()` | function | PyJWKClient JWT verification |
| `_read_json_body()` | async function | JSON body parsing with size limit (1MB) |

## Routes
| Path | Methods | Description |
|------|---------|-------------|
| `/` | GET | Server info |
| `/health` | GET | Health check |
| `/dashboard` | GET | HTML dashboard |
| `/.well-known/agent-card.json` | GET | A2A agent card |
| `/.well-known/agent.json` | GET | A2A agent card |
| `/.well-known/oauth-client.json` | GET | OAuth 2.1 client metadata |
| `/.well-known/oauth-authorization-server` | GET | OAuth 2.1 AS metadata |
| `/a2a/v1` | POST | A2A JSON-RPC endpoint |
| `/mcp` | GET, POST, DELETE | MCP streamable HTTP endpoint |

## Security Middleware
1. **CORS** — Origin whitelist via `SIMONE_ALLOWED_ORIGINS`
2. **Auth** — Bearer JWT validation for non-open paths
3. **Rate Limit** — Per-IP sliding window (100 req/60s, configurable)
4. **Body Limit** — 1MB max request body
5. **Path Validation** — Open paths bypass auth (/, /health, /dashboard, .well-known)

## Relationship
- `src/simone_mcp/core.py` — `build_agent_card()`, `dashboard()`, `OPEN_PATHS`
- `src/simone_mcp/a2a_handler.py` — `handle_a2a_request()` for POST /a2a/v1
- `src/simone_mcp/protocol.py` — `handle_mcp_request()` for POST /mcp
- `src/simone_mcp/hybrid_memory.py` — `shutdown_stores()` for lifespan cleanup
- `src/simone_mcp/correlation.py` — `correlation_manager` for tool call tracking
- `src/main.py` — imports `create_app()`
- `tests/test_simone_mcp.py` — tests all security middleware and routes

## Dependencies
- `fastapi` — Web framework
- `starlette` — CORS middleware
- `pyjwt` — JWT verification
- `a2a_handler`: `handle_a2a_request`
- `protocol`: `handle_mcp_request`, `_log_event`, `_get_events_after`, `_remove_session`
- `core`: `build_agent_card`, `build_oauth_client_metadata`, `build_authorization_server_metadata`, `dashboard`, `OPEN_PATHS`, `MCP_ENDPOINT`, `A2A_ENDPOINT`
- `hybrid_memory`: `shutdown_stores`
- `correlation`: `correlation_manager`

## Environment Variables
| Variable | Default | Purpose |
|----------|---------|---------|
| `SIMONE_RATE_LIMIT_WINDOW` | 60 | Rate limit window in seconds |
| `SIMONE_RATE_LIMIT_MAX` | 100 | Max requests per window |
| `SIMONE_MAX_REQUEST_BODY` | 1048576 | Max request body bytes |
| `SIMONE_ALLOWED_ORIGINS` | localhost, 127.0.0.1, opensin.ai | CORS whitelist |
| `SIMONE_AUTH_REQUIRED` | false | Enable OAuth 2.1 |
| `SIMONE_OAUTH_JWKS_URL` | — | JWKS endpoint |
| `SIMONE_OAUTH_ISSUER` | — | Token issuer |
| `SIMONE_OAUTH_AUDIENCE` | simone-mcp | Token audience |
| `SIMONE_OAUTH_ALGORITHMS` | RS256,ES256 | Accepted algorithms |

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
