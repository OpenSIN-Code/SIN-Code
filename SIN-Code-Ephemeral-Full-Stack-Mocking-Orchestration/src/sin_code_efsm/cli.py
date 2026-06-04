"""CLI fuer den Ephemeral Mock Orchestrator."""
from __future__ import annotations

import json
import threading

import typer

from .orchestrator import EphemeralOrchestrator

app = typer.Typer(help="SIN-Code Ephemeral Mock Orchestrator CLI")


@app.command()
def setup(
    name: str,
    apis: list[str] = typer.Option([], "--api", help="External API to mock"),
    requires_db: bool = typer.Option(False, "--db"),
    test_cmd: str = typer.Option("pytest", "--test-cmd"),
    serve_mock: bool = typer.Option(False, "--serve-mock", help="Run HTTP mock server"),
):
    """Spin up ephemeral environment and run tests."""
    orch = EphemeralOrchestrator()
    ctx = {
        "name": name,
        "external_apis": apis,
        "requires_db": requires_db,
        "test_command": test_cmd,
    }
    env = orch.configure_from_task(ctx)
    summary = {
        "mock_port": env.mock_port,
        "db_dsn": env.db_dsn,
        "env_vars": env.env_vars,
        "sandbox_backend": env.sandbox_backend,
    }
    typer.echo("[EFSM] Environment prepared:")
    typer.echo(json.dumps(summary, indent=2))

    if serve_mock and apis:
        thread = threading.Thread(target=orch.mock_server.run, daemon=True)
        thread.start()
        typer.echo(f"[EFSM] Mock server started on port {env.mock_port}")

    typer.echo(f"[EFSM] Running: {test_cmd}")
    result = orch.run_tests(test_cmd, with_network=bool(apis))
    typer.echo(json.dumps(result, indent=2))


if __name__ == "__main__":
    app()
