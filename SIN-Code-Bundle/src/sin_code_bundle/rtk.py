"""RTK bridge.

RTK (https://github.com/rtk-ai/rtk) is an *upstream* tool distributed as an
Apache-2.0 single Rust binary. It is a CLI proxy that filters and compresses
command output (ls, grep, git, test runners, ...) before it reaches an LLM,
cutting token consumption by 60-90%.

Unlike GitNexus or MarkItDown, RTK is **not** an MCP server: it integrates with
each coder agent through that agent's own hook / plugin mechanism, installed by
RTK's native ``rtk init`` command. We therefore never vendor RTK; the bridge
simply discovers the upstream ``rtk`` binary and drives ``rtk init`` for each
agent so the whole SIN-Code coder fleet benefits from the same token savings.
"""

from __future__ import annotations

import shutil
import subprocess
from dataclasses import dataclass
from typing import Any

RTK_BINARY = "rtk"

# How RTK wires itself into each supported coder agent. Mirrors the upstream
# `rtk init` matrix (see RTK README "Supported AI Tools").
_INIT_ARGS: dict[str, list[str]] = {
    "opencode": ["init", "-g", "--opencode"],
    "codex": ["init", "-g", "--codex"],
    "hermes": ["init", "--agent", "hermes"],
}

AGENTS = tuple(_INIT_ARGS.keys())


class RtkError(RuntimeError):
    """Raised when RTK is unavailable or an init command fails."""


@dataclass
class RtkEnv:
    """Resolved runtime environment for invoking RTK."""

    rtk: str | None

    @property
    def available(self) -> bool:
        return bool(self.rtk)

    def base_cmd(self) -> str:
        if not self.rtk:
            raise RtkError(
                "`rtk` not found on PATH. Install it with `brew install rtk`, "
                "`cargo install --git https://github.com/rtk-ai/rtk`, or the "
                "install script at https://github.com/rtk-ai/rtk. The bundle "
                "does not vendor RTK."
            )
        return self.rtk


def detect_env() -> RtkEnv:
    return RtkEnv(rtk=shutil.which(RTK_BINARY))


def init_args(agent: str) -> list[str]:
    """Return the upstream ``rtk init`` arguments for an agent."""
    try:
        return list(_INIT_ARGS[agent])
    except KeyError:
        raise RtkError(f"Unknown agent: {agent!r}. Known: {', '.join(AGENTS)}")


def _run(cmd: list[str], timeout: int = 120) -> str:
    try:
        proc = subprocess.run(cmd, capture_output=True, text=True, timeout=timeout)
    except FileNotFoundError as exc:  # pragma: no cover - guarded by detect_env
        raise RtkError(f"Failed to execute {cmd[0]!r}: {exc}") from exc
    except subprocess.TimeoutExpired as exc:  # pragma: no cover - timing dependent
        raise RtkError(f"rtk timed out after {timeout}s") from exc
    if proc.returncode != 0:
        raise RtkError(f"`{' '.join(cmd)}` failed ({proc.returncode}): {proc.stderr.strip()}")
    return proc.stdout.strip()


def setup_agents(
    agents: list[str] | None = None,
    env: RtkEnv | None = None,
) -> dict[str, str]:
    """Run ``rtk init`` for each agent so it intercepts/compacts their commands.

    Returns a mapping of agent -> the rtk command that was executed.
    """
    env = env or detect_env()
    rtk = env.base_cmd()
    chosen = agents or list(AGENTS)
    done: dict[str, str] = {}
    for agent in chosen:
        cmd = [rtk, *init_args(agent)]
        _run(cmd)
        done[agent] = " ".join(cmd)
    return done


def gain(env: RtkEnv | None = None) -> dict[str, Any]:
    """Return RTK's token-savings stats as JSON (best-effort)."""
    env = env or detect_env()
    rtk = env.base_cmd()
    out = _run([rtk, "gain", "--all", "--format", "json"])
    try:
        import json

        return json.loads(out or "{}")
    except (ValueError, TypeError):
        return {"raw": out}


def doctor() -> dict[str, Any]:
    """Report RTK availability for diagnostics."""
    env = detect_env()
    return {
        "available": env.available,
        "binary": env.rtk,
        "agents": list(AGENTS),
    }
