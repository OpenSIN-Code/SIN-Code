"""Stateful Mock-Server fuer externe APIs."""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass
class StatefulMock:
    """Mock mit echtem Zustand."""

    name: str
    base_path: str
    state: dict[str, Any] = field(default_factory=dict)
    scenarios: dict[str, dict] = field(default_factory=dict)

    def respond(self, method: str, path: str, body: dict | None = None) -> dict:
        key = f"{method}:{path}"
        if key in self.scenarios:
            return self.scenarios[key]
        resource = path.strip("/").split("/")[-1] or "root"
        if method == "POST":
            self.state.setdefault(resource, []).append(body)
            return {
                "status": "created",
                "id": len(self.state[resource]),
                "data": body,
            }
        if method == "GET":
            return {"data": self.state.get(resource, [])}
        if method == "DELETE":
            self.state.pop(resource, None)
            return {"status": "deleted", "resource": resource}
        if method in ("PUT", "PATCH"):
            self.state[resource] = body
            return {"status": "updated", "data": body}
        return {"status": "ok"}


class MockServer:
    """Mock-Server mit mehreren Stateful-Mocks.

    Der FastAPI-Layer ist optional: ist FastAPI nicht installiert, kann der
    Server trotzdem in-process via ``dispatch`` getestet werden.
    """

    def __init__(self) -> None:
        self.mocks: dict[str, StatefulMock] = {}
        self._app = None

    def add_mock(self, mock: StatefulMock) -> None:
        self.mocks[mock.base_path] = mock

    def dispatch(self, method: str, path: str, body: dict | None = None) -> dict:
        """Routet eine Anfrage ohne HTTP-Layer (fuer Tests/in-process)."""
        normalized = path if path.startswith("/") else f"/{path}"
        for base, mock in self.mocks.items():
            prefix = base if base.startswith("/") else f"/{base}"
            if normalized.startswith(prefix):
                return mock.respond(method, normalized, body)
        return {"error": "no mock matched", "path": normalized}

    @property
    def app(self):
        if self._app is None:
            self._app = self._build_app()
        return self._app

    def _build_app(self):
        try:
            from fastapi import FastAPI, Request
            from fastapi.responses import JSONResponse
        except ImportError as exc:  # pragma: no cover
            raise RuntimeError(
                "FastAPI is required to run the HTTP mock server. "
                "Install with: pip install fastapi uvicorn"
            ) from exc

        app = FastAPI(title="SIN-Code Ephemeral Mock Server")

        @app.api_route(
            "/{path:path}",
            methods=["GET", "POST", "PUT", "DELETE", "PATCH"],
        )
        async def catch_all(request: Request, path: str):  # noqa: ANN001
            body = None
            if request.method in ("POST", "PUT", "PATCH"):
                try:
                    body = await request.json()
                except Exception:
                    body = None
            result = self.dispatch(request.method, f"/{path}", body)
            status = 404 if result.get("error") == "no mock matched" else 200
            return JSONResponse(result, status_code=status)

        return app

    def run(self, host: str = "127.0.0.1", port: int = 8787) -> None:  # pragma: no cover
        import uvicorn

        uvicorn.run(self.app, host=host, port=port)
