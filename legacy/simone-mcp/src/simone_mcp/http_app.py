from __future__ import annotations

import asyncio
import json
import os
import uuid
from typing import Any

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import HTMLResponse, JSONResponse, StreamingResponse
import jwt

from .core import (
    A2A_ENDPOINT,
    MCP_ENDPOINT,
    OPEN_PATHS,
    TOOL_DEFINITIONS,
    build_agent_card,
    build_authorization_server_metadata,
    build_oauth_client_metadata,
    dashboard,
    execute_simone_action,
    json_dumps,
)


def _base_url(request: Request) -> str:
    return str(request.base_url).rstrip("/")


def _allowed_origins() -> set[str]:
    configured = os.getenv(
        "SIMONE_ALLOWED_ORIGINS",
        "http://localhost,http://127.0.0.1,https://a2a.delqhi.com",
    )
    return {value.strip() for value in configured.split(",") if value.strip()}


def _should_require_auth() -> bool:
    return os.getenv("SIMONE_AUTH_REQUIRED", "false").lower() in {
        "1",
        "true",
        "yes",
        "on",
    }


def _verify_token(token: str) -> dict[str, Any]:
    audience = os.getenv("SIMONE_OAUTH_AUDIENCE", "simone-mcp")
    issuer = os.getenv("SIMONE_OAUTH_ISSUER")
    jwks_url = os.getenv("SIMONE_OAUTH_JWKS_URL")
    algorithms = [
        value.strip()
        for value in os.getenv("SIMONE_OAUTH_ALGORITHMS", "RS256,ES256").split(",")
        if value.strip()
    ]
    if not jwks_url:
        raise HTTPException(status_code=401, detail="jwks_not_configured")
    client = jwt.PyJWKClient(jwks_url)
    signing_key = client.get_signing_key_from_jwt(token)
    return jwt.decode(
        token,
        signing_key.key,
        algorithms=algorithms,
        audience=audience,
        issuer=issuer,
        options={"verify_aud": bool(audience), "verify_iss": bool(issuer)},
    )


def _authorize_request(request: Request) -> dict[str, Any] | None:
    if request.url.path in OPEN_PATHS:
        return None
    if not _should_require_auth():
        return None
    header = request.headers.get("authorization", "")
    scheme, _, token = header.partition(" ")
    if scheme.lower() != "bearer" or not token:
        raise HTTPException(status_code=401, detail="missing_bearer_token")
    try:
        return _verify_token(token)
    except HTTPException:
        raise
    except Exception as error:
        raise HTTPException(status_code=401, detail=f"invalid_token:{error}") from error


def _validate_origin(request: Request) -> None:
    origin = request.headers.get("origin")
    if not origin:
        return
    if origin not in _allowed_origins():
        raise HTTPException(status_code=403, detail="origin_not_allowed")


async def _mcp_post(request: Request) -> JSONResponse | StreamingResponse:
    payload = await request.json()
    method = payload.get("method")
    request_id = payload.get("id")
    session_id = request.headers.get("Mcp-Session-Id") or str(uuid.uuid4())
    headers = {"Mcp-Session-Id": session_id}
    if method == "initialize":
        result = {
            "protocolVersion": "2025-03-26",
            "capabilities": {"tools": {}, "logging": {}},
            "serverInfo": {"name": "simone-mcp", "version": "2026.04.12"},
        }
        return JSONResponse(
            {"jsonrpc": "2.0", "id": request_id, "result": result}, headers=headers
        )
    if method == "tools/list":
        return JSONResponse(
            {"jsonrpc": "2.0", "id": request_id, "result": {"tools": TOOL_DEFINITIONS}},
            headers=headers,
        )
    if method == "tools/call":
        params = payload.get("params", {})
        name = params.get("name")
        arguments = params.get("arguments", {})
        action = dict(arguments)
        action["action"] = name
        result = await execute_simone_action(action)
        return JSONResponse(
            {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "content": [{"type": "text", "text": json_dumps(result)}],
                    "isError": not result.get("ok", False),
                },
            },
            headers=headers,
        )
    if method == "notifications/initialized":
        return JSONResponse({}, status_code=202, headers=headers)
    return JSONResponse(
        {
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {"code": -32601, "message": "Method not found"},
        },
        status_code=404,
        headers=headers,
    )


async def _mcp_get(request: Request) -> StreamingResponse:
    session_id = request.headers.get("Mcp-Session-Id") or str(uuid.uuid4())

    async def event_stream() -> Any:
        yield "event: ready\n"
        yield f"id: {session_id}:1\n"
        yield "data: {}\n\n"
        await asyncio.sleep(0)

    return StreamingResponse(
        event_stream(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "Mcp-Session-Id": session_id},
    )


def create_app() -> FastAPI:
    app = FastAPI(title="Simone MCP", version="2026.04.12")

    @app.middleware("http")
    async def security_middleware(request: Request, call_next):
        _validate_origin(request)
        _authorize_request(request)
        return await call_next(request)

    @app.get("/")
    async def root() -> dict[str, Any]:
        return {"ok": True, "name": "simone-mcp"}

    @app.get("/health")
    async def health() -> dict[str, Any]:
        return {"ok": True, "status": "ok", "name": "simone-mcp"}

    @app.get("/dashboard", response_class=HTMLResponse)
    async def dashboard_view() -> str:
        return await dashboard()

    @app.get("/.well-known/agent-card.json")
    async def well_known_card(request: Request) -> dict[str, Any]:
        return build_agent_card(_base_url(request))

    @app.get("/.well-known/agent.json")
    async def well_known_agent(request: Request) -> dict[str, Any]:
        return build_agent_card(_base_url(request))

    @app.get("/.well-known/oauth-client.json")
    async def well_known_client(request: Request) -> dict[str, Any]:
        return build_oauth_client_metadata(_base_url(request))

    @app.get("/.well-known/oauth-authorization-server")
    async def oauth_authorization_server(request: Request) -> dict[str, Any]:
        return build_authorization_server_metadata(_base_url(request))

    @app.post(A2A_ENDPOINT)
    async def a2a_rpc(request: Request) -> JSONResponse:
        payload = await request.json()
        method = payload.get("method")
        request_id = payload.get("id")
        if method == "agent/getCard":
            return JSONResponse(
                {
                    "jsonrpc": "2.0",
                    "id": request_id,
                    "result": build_agent_card(_base_url(request)),
                }
            )
        if method == "message/send":
            params = payload.get("params", {})
            parts = params.get("message", {}).get("parts", [])
            text = " ".join(str(part.get("text", "")) for part in parts).strip()
            action = {"action": text or "agent.help"}
            try:
                parsed = json.loads(text) if text else None
                if isinstance(parsed, dict) and isinstance(parsed.get("action"), str):
                    action = parsed
            except json.JSONDecodeError:
                pass
            result = await execute_simone_action(action)
            body = {
                "jsonrpc": "2.0",
                "id": request_id,
                "result": {
                    "id": str(uuid.uuid4()),
                    "kind": "task",
                    "status": {
                        "state": "completed",
                        "message": {
                            "role": "agent",
                            "parts": [{"type": "text", "text": "completed"}],
                        },
                    },
                    "artifacts": [
                        {
                            "id": str(uuid.uuid4()),
                            "name": action.get("action", "agent.help"),
                            "parts": [{"type": "data", "data": result}],
                        }
                    ],
                },
            }
            return JSONResponse(body)
        return JSONResponse(
            {
                "jsonrpc": "2.0",
                "id": request_id,
                "error": {"code": -32601, "message": "Method not found"},
            },
            status_code=404,
        )

    @app.api_route(MCP_ENDPOINT, methods=["GET", "POST", "DELETE"])
    async def mcp_endpoint(request: Request):
        if request.method == "GET":
            return await _mcp_get(request)
        if request.method == "DELETE":
            session_id = request.headers.get("Mcp-Session-Id")
            if not session_id:
                raise HTTPException(status_code=400, detail="missing_session_id")
            return JSONResponse({}, status_code=202)
        return await _mcp_post(request)

    return app
