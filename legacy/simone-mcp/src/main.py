"""ASGI entry point: builds the FastAPI app for uvicorn / gunicorn.

`uvicorn src.main:app --host 0.0.0.0 --port 8234` is the canonical
production launch command. The app exposes:
  - `/mcp`        — MCP streamable HTTP transport
  - `/a2a/v1`     — A2A JSON-RPC
  - `/health`     — readiness check
  - `/dashboard`  — HTML status page

Docs: main.doc.md
"""
from .simone_mcp.http_app import create_app


app = create_app()