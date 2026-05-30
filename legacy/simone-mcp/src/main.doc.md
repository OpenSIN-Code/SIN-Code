# `src/main.py` — FastAPI Application Factory

Partner file: `src/main.py`

## Purpose
FastAPI ASGI application entry point. Creates the `app` instance via `http_app.create_app()` for use with ASGI servers (uvicorn, gunicorn, etc.).

## Key Symbols
- `app` — FastAPI instance exposed to ASGI servers

## Relationship
- `src/simone_mcp/http_app.py` — `create_app()` function
- `src/simone_mcp/core.py` — underlying tool implementations
- `src/simone_mcp/protocol.py` — MCP protocol handlers

## Dependencies
- `simone_mcp.http_app.create_app`

## Usage
```bash
uvicorn src.main:app --host 0.0.0.0 --port 8234
```
