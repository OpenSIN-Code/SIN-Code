"""Sandbox fuer isolierte Agent-Execution.

Bevorzugt Docker; faellt auf einen lokalen Subprozess zurueck, wenn Docker
nicht verfuegbar ist (mit reduzierter Isolation, klar gekennzeichnet).
"""
from __future__ import annotations

import shlex
import subprocess
from dataclasses import dataclass


@dataclass
class SandboxResult:
    exit_code: int
    stdout: str
    stderr: str
    container_id: str | None = None
    backend: str = "docker"


def docker_available() -> bool:
    """Prueft, ob ein nutzbarer Docker-Daemon erreichbar ist."""
    try:
        import docker
    except ImportError:
        return False
    try:
        client = docker.from_env()
        client.ping()
        return True
    except Exception:
        return False


class DockerSandbox:
    """Isolierter Container fuer Agent-Code (mit Subprozess-Fallback)."""

    def __init__(self, base_image: str = "python:3.11-slim", allow_fallback: bool = True):
        self.base_image = base_image
        self.allow_fallback = allow_fallback
        self._client = None

    def _client_init(self):
        if self._client is None:
            import docker

            self._client = docker.from_env()
        return self._client

    def _run_subprocess(self, command: str, timeout: int) -> SandboxResult:
        try:
            proc = subprocess.run(
                command,
                shell=True,
                capture_output=True,
                text=True,
                timeout=timeout,
            )
            return SandboxResult(
                exit_code=proc.returncode,
                stdout=proc.stdout,
                stderr=proc.stderr,
                backend="subprocess",
            )
        except subprocess.TimeoutExpired:
            return SandboxResult(
                exit_code=124,
                stdout="",
                stderr=f"Timeout after {timeout}s",
                backend="subprocess",
            )

    def run_command(
        self,
        command: str,
        timeout: int = 60,
        network: bool = False,
        extra_hosts: dict[str, str] | None = None,
    ) -> SandboxResult:
        if not docker_available():
            if not self.allow_fallback:
                raise RuntimeError("Docker not available and fallback disabled")
            return self._run_subprocess(command, timeout)

        client = self._client_init()
        kwargs: dict = {
            "command": ["sh", "-c", command],
            "detach": True,
            "mem_limit": "512m",
            "network_mode": "bridge" if network else "none",
        }
        if extra_hosts:
            kwargs["extra_hosts"] = extra_hosts
        container = client.containers.run(self.base_image, **kwargs)
        try:
            result = container.wait(timeout=timeout)
            return SandboxResult(
                exit_code=result.get("StatusCode", -1),
                stdout=container.logs(stdout=True, stderr=False).decode(errors="replace"),
                stderr=container.logs(stdout=False, stderr=True).decode(errors="replace"),
                container_id=container.id,
                backend="docker",
            )
        finally:
            try:
                container.remove(force=True)
            except Exception:
                pass
