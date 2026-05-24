from __future__ import annotations

import asyncio
import json
import logging
import os
import threading
import time
import uuid
from collections import defaultdict
from typing import Any

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import HTMLResponse, JSONResponse, StreamingResponse
import jwt

from .a2a_handler import handle_a2a_request
from .correlation import correlation_manager
from .core import (
    A2A_ENDPOINT,
    MCP_ENDPOINT,
    OPEN_PATHS,
    build_agent_card,
    build_authorization_server_metadata,
    build_oauth_client_metadata,
    dashboard,
)
from .hybrid_memory import shutdown_stores
from .protocol import handle_mcp_request, _log_event, _get_events_after, PROTOCOL_VERSION, SSE_RETRY_MS, _remove_session

logger = logging.getLogger(__name__)

_RATE_LIMIT_WINDOW = int(os.getenv("SIMONE_RATE_LIMIT_WINDOW", "60"))
_RATE_LIMIT_MAX = int(os.getenv("SIMONE_RATE_LIMIT_MAX", "100"))
_RATE_LIMIT_CLEANUP_EVERY = 128
_MAX_REQUEST_BODY = int(os.getenv("SIMONE_MAX_REQUEST_BODY", "1048576"))
_rate_limit_store: dict[str, list[float]] = defaultdict(list)
_rate_limit_op_count = 0
_rate_limit_lock = threading.Lock()

_allowed_origins_cache: set[str] | None = None
_allowed_origins_lock = threading.Lock()
_auth_required_cache: bool | None = None


def _get_allowed_origins() -> set[str]:
    global _allowed_origins_cache
    if _allowed_origins_cache is not None:
        return _allowed_origins_cache
    with _allowed_origins_lock:
        if _allowed_origins_cache is not None:
            return _allowed_origins_cache
        configured = os.getenv(
            "SIMONE_ALLOWED_ORIGINS",
            "http://localhost,http://127.0.0.1,https://opensin.ai",
        )
        _allowed_origins_cache = {v.strip() for v in configured.split(",") if v.strip()}
        return _allowed_origins_cache


def _should_require_auth() -> bool:
    global _auth_required_cache
    if _auth_required_cache is not None:
        return _auth_required_cache
    _auth_required_cache = os.getenv("SIMONE_AUTH_REQUIRED", "false").lower() in {
        "1", "true", "yes", "on",
    }
    return _auth_required_cache


def _check_rate_limit(client_id: str) -> None:
    global _rate_limit_op_count
    with _rate_limit_lock:
        now = time.monotonic()
        window = _rate_limit_store[client_id]
        window[:] = [t for t in window if now - t < _RATE_LIMIT_WINDOW]
        if len(window) >= _RATE_LIMIT_MAX:
            oldest = window[0]
            retry_after = int(_RATE_LIMIT_WINDOW - (now - oldest)) + 1
            raise HTTPException(
                status_code=429,
                detail="rate_limit_exceeded",
                headers={"Retry-After": str(retry_after)},
            )
        window.append(now)
        _rate_limit_op_count += 1
        if _rate_limit_op_count % _RATE_LIMIT_CLEANUP_EVERY == 0:
            stale = [
                cid for cid, ts in _rate_limit_store.items()
                if all(now - t > _RATE_LIMIT_WINDOW for t in ts)
            ]
            for cid in stale:
                del _rate_limit_store[cid]


def _extract_client_ip(request: Request) -> str:
    forwarded = request.headers.get("x-forwarded-for", "")
    if forwarded:
        rightmost = forwarded.rsplit(",", 1)[-1].strip()
        if rightmost:
            return str(rightmost)
    return request.client.host if request.client else "unknown"  # type: ignore[no-any-return]


def _base_url(request: Request) -> str:
    return str(request.base_url).rstrip("/")


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
    return jwt.decode(  # type: ignore[no-any-return]
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
    if origin not in _get_allowed_origins():
        raise HTTPException(status_code=403, detail="origin_not_allowed")


async def _read_json_body(request: Request) -> dict[str, Any] | list[Any]:
    body = await request.body()
    if len(body) > _MAX_REQUEST_BODY:
        raise HTTPException(status_code=413, detail="request_body_too_large")
    try:
        return json.loads(body)  # type: ignore[no-any-return]
    except json.JSONDecodeError as e:
        raise HTTPException(status_code=400, detail=f"invalid_json: {e}") from e


async def _send_sse_notification(response: StreamingResponse | None, notification: dict[str, Any]) -> None:
    pass


async def _mcp_post(request: Request) -> JSONResponse | StreamingResponse:
    raw_payload = await _read_json_body(request)
    session_id = request.headers.get("Mcp-Session-Id") or None
    client_protocol_version = request.headers.get("MCP-Protocol-Version")

    mcp_method_header = request.headers.get("Mcp-Method")
    mcp_name_header = request.headers.get("Mcp-Name")
    mcp_param_headers: dict[str, str] = {}
    for hdr_key, hdr_val in request.headers.items():
        lower_key = hdr_key.lower()
        if lower_key.startswith("mcp-param-"):
            param_name = hdr_key[len("mcp-param-"):]
            mcp_param_headers[param_name] = hdr_val

    payloads = raw_payload if isinstance(raw_payload, list) else [raw_payload]
    responses = []
    all_notifications = []

    for payload in payloads:
        if not isinstance(payload, dict):
            responses.append({
                "jsonrpc": "2.0",
                "id": None,
                "error": {"code": -32600, "message": "Request must be a JSON object"},
            })
            continue

        if mcp_method_header:
            body_method = payload.get("method", "")
            if body_method and body_method != mcp_method_header:
                responses.append({
                    "jsonrpc": "2.0",
                    "id": payload.get("id"),
                    "error": {"code": -32001, "message": f"HeaderMismatch: Mcp-Method header '{mcp_method_header}' does not match body method '{body_method}'"},
                })
                continue

        name = ""
        raw_params = payload.get("params")
        params: dict[str, Any] = raw_params if isinstance(raw_params, dict) else {}
        if payload.get("method") == "tools/call":
            name = params.get("name", "")

        if mcp_name_header and payload.get("method") == "tools/call":
            if name and name != mcp_name_header:
                responses.append({
                    "jsonrpc": "2.0",
                    "id": payload.get("id"),
                    "error": {"code": -32001, "message": f"HeaderMismatch: Mcp-Name header '{mcp_name_header}' does not match tool name '{name}'"},
                })
                continue

        if mcp_param_headers and payload.get("method") == "tools/call":
            arguments = params.get("arguments") or {}
            header_mismatch = False
            for param_name, header_val in mcp_param_headers.items():
                body_val = arguments.get(param_name)
                if body_val is not None and str(body_val) != header_val:
                    responses.append({
                        "jsonrpc": "2.0",
                        "id": payload.get("id"),
                        "error": {"code": -32001, "message": f"HeaderMismatch: Mcp-Param-{param_name} header '{header_val}' does not match body argument value '{body_val}'"},
                    })
                    header_mismatch = True
                    break
            if header_mismatch:
                continue

        correlation_id = None
        if name:
            arguments = params.get("arguments") or {}
            tool_call_id = params.get("_meta", {}).get("tool_call_id") if isinstance(params.get("_meta"), dict) else None
            correlation_id = correlation_manager.generate_correlation_id(name, arguments, tool_call_id)

        response, new_session_id, notifications = await handle_mcp_request(
            payload, session_id, send_notification=None, client_protocol_version=client_protocol_version
        )
        if new_session_id:
            session_id = new_session_id

        all_notifications.extend(notifications)

        if response is not None:
            if correlation_id and "result" in response and isinstance(response["result"], dict):
                response["result"]["_meta"] = response["result"].get("_meta", {})
                response["result"]["_meta"]["correlation_id"] = correlation_id
                if name:
                    correlation_manager.complete_call(correlation_id, response["result"])
            elif correlation_id and "error" in response:
                correlation_manager.complete_call(correlation_id, None, str(response["error"]))
            responses.append(response)

    headers = {}
    if session_id:
        headers["Mcp-Session-Id"] = session_id
    headers["MCP-Protocol-Version"] = PROTOCOL_VERSION

    if all_notifications:
        for n in all_notifications:
            if session_id:
                _log_event(session_id, str(uuid.uuid4()), n)

    if isinstance(raw_payload, list):
        has_requests = any(p.get("method") not in {"notifications/initialized", "initialized"} for p in payloads if isinstance(p, dict))
        if not has_requests:
            return JSONResponse({}, status_code=202, headers=headers)
        return JSONResponse(responses, headers=headers)

    if not responses:
        return JSONResponse({}, status_code=202, headers=headers)

    single = responses[0]
    if single.get("error") and isinstance(single["error"], dict):
        status = 400
        if single["error"].get("code") == -32601:
            status = 404
        elif single["error"].get("code") == -32002:
            status = 400
        return JSONResponse(single, status_code=status, headers=headers)
    return JSONResponse(single, headers=headers)


async def _mcp_get(request: Request) -> StreamingResponse:
    session_id = request.headers.get("Mcp-Session-Id") or str(uuid.uuid4())
    last_event_id = request.headers.get("Last-Event-ID")
    event_counter = 0

    async def event_stream() -> Any:
        nonlocal event_counter
        if last_event_id and session_id:
            replay = _get_events_after(session_id, last_event_id)
            for evt in replay:
                event_counter += 1
                eid = f"{session_id}:{event_counter}"
                yield f"id: {eid}\n"
                yield f"data: {json.dumps(evt['data'])}\n\n"

        event_counter += 1
        yield f"retry: {SSE_RETRY_MS}\n"
        yield f"id: {session_id}:{event_counter}\n"
        yield "event: ready\n"
        yield "data: {}\n\n"

        try:
            while True:
                await asyncio.sleep(30)
                event_counter += 1
                eid = f"{session_id}:{event_counter}"
                yield f"id: {eid}\n"
                yield "event: ping\n"
                yield "data: {}\n\n"
        except asyncio.CancelledError:
            pass

    return StreamingResponse(
        event_stream(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "Mcp-Session-Id": session_id},
    )


def create_app() -> FastAPI:
    from contextlib import asynccontextmanager

    @asynccontextmanager
    async def lifespan(app: FastAPI):
        logger.info("Simone MCP starting up")
        yield
        logger.info("Simone MCP shutting down")
        shutdown_stores()

    app = FastAPI(title="Simone MCP", version="2026.06.30", lifespan=lifespan)

    from starlette.middleware.cors import CORSMiddleware

    origins = list(_get_allowed_origins())
    app.add_middleware(
        CORSMiddleware,
        allow_origins=origins,
        allow_methods=["GET", "POST", "DELETE"],
        allow_headers=["Authorization", "Content-Type", "Mcp-Session-Id", "MCP-Protocol-Version", "Mcp-Method", "Mcp-Name"],
        max_age=3600,
    )

    @app.middleware("http")
    async def security_middleware(request: Request, call_next):
        _validate_origin(request)
        _authorize_request(request)
        if request.url.path not in OPEN_PATHS:
            client_id = _extract_client_ip(request)
            _check_rate_limit(client_id)
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
        payload = await _read_json_body(request)
        if not isinstance(payload, dict):
            raise HTTPException(status_code=400, detail="invalid_payload")
        base_url = _base_url(request)
        result = await handle_a2a_request(payload, base_url)
        return JSONResponse(result)

    @app.api_route(MCP_ENDPOINT, methods=["GET", "POST", "DELETE"])
    async def mcp_endpoint(request: Request):
        if request.method == "GET":
            return await _mcp_get(request)
        if request.method == "DELETE":
            session_id = request.headers.get("Mcp-Session-Id")
            if not session_id:
                raise HTTPException(status_code=400, detail="missing_session_id")
            _remove_session(session_id)
            return JSONResponse({}, status_code=202)
        return await _mcp_post(request)

    return app
