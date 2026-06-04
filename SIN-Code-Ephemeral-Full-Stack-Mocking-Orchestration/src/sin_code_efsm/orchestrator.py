"""Orchestriert Mock-Server und Sandbox fuer einen Agent-Task."""
from __future__ import annotations

from dataclasses import dataclass, field

from .mock_generator import MockServer, StatefulMock
from .sandbox import DockerSandbox, docker_available


@dataclass
class TestEnvironment:
    mock_port: int
    db_dsn: str | None
    container_id: str | None
    env_vars: dict[str, str] = field(default_factory=dict)
    sandbox_backend: str = "docker"


class EphemeralOrchestrator:
    """Baut eine vollstaendige Testumgebung fuer einen Agent-Task."""

    def __init__(self, mock_port: int = 8787):
        self.mock_server = MockServer()
        self.sandbox = DockerSandbox()
        self._mock_port = mock_port

    def configure_from_task(self, task_context: dict) -> TestEnvironment:
        """task_context keys: name, external_apis, requires_db, test_command."""
        env_vars: dict[str, str] = {}
        # Bei Docker erreicht der Container den Host ueber host.docker.internal,
        # bei Subprozess-Fallback ueber localhost.
        host = "host.docker.internal" if docker_available() else "127.0.0.1"
        for api in task_context.get("external_apis", []):
            self.mock_server.add_mock(StatefulMock(name=api, base_path=f"/{api}"))
            env_vars[f"{api.upper()}_BASE_URL"] = (
                f"http://{host}:{self._mock_port}/{api}"
            )

        db_dsn = None
        if task_context.get("requires_db"):
            db_dsn = "sqlite:///:memory:"
            env_vars["DATABASE_URL"] = db_dsn

        return TestEnvironment(
            mock_port=self._mock_port,
            db_dsn=db_dsn,
            container_id=None,
            env_vars=env_vars,
            sandbox_backend="docker" if docker_available() else "subprocess",
        )

    def run_tests(self, test_command: str, with_network: bool = True) -> dict:
        extra_hosts = (
            {"host.docker.internal": "host-gateway"} if with_network else None
        )
        try:
            result = self.sandbox.run_command(
                test_command,
                timeout=120,
                network=with_network,
                extra_hosts=extra_hosts,
            )
            return {
                "exit_code": result.exit_code,
                "stdout": result.stdout[:2000],
                "stderr": result.stderr[:2000],
                "success": result.exit_code == 0,
                "backend": result.backend,
            }
        except Exception as exc:
            return {
                "exit_code": -1,
                "stdout": "",
                "stderr": str(exc),
                "success": False,
                "backend": "none",
            }
