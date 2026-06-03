"""GitNexus bridge.

GitNexus (https://github.com/abhigyanpatwari/GitNexus) is an *upstream* tool,
distributed as the npm package ``gitnexus`` under the PolyForm Noncommercial
license. We never vendor or copy its source; we only invoke the published
package via ``npx`` and read the artifacts it produces. This keeps the bundle
MIT-licensed while making GitNexus a hard, always-on dependency so that coder
agents never operate "blind" on a repository.

The bridge provides:
  * discovery / health checks for Node + the ``gitnexus`` package,
  * an ``ensure_index`` helper that auto-indexes a repo when the graph is
    missing or stale,
  * thin wrappers over the GitNexus CLI query surface
    (``ai-context``, ``query``, ``context``, ``impact``),
  * MCP wiring so OpenCode / Codex / Hermes each get the GitNexus MCP server.

Docs: gitnexus.doc.md
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

# How GitNexus is provided. We always pin to the published package and let npx
# fetch/cache it, mirroring GitNexus' own `.mcp.json` recommendation.
GITNEXUS_PACKAGE = "gitnexus@latest"

# Re-index when the stored graph is older than this many seconds (default 24h).
DEFAULT_STALE_SECONDS = 24 * 60 * 60

# GitNexus stores its index per-repo under this directory.
INDEX_DIRNAME = ".gitnexus"


class GitNexusError(RuntimeError):
    """Raised when GitNexus is unavailable or a command fails."""


@dataclass
class GitNexusEnv:
    """Resolved runtime environment for invoking GitNexus."""

    node: str | None
    npx: str | None
    package: str = GITNEXUS_PACKAGE

    @property
    def available(self) -> bool:
        return bool(self.npx)

    def base_cmd(self) -> list[str]:
        if not self.npx:
            raise GitNexusError(
                "npx not found on PATH. GitNexus requires Node.js (>=18). "
                "Install Node, then re-run. The bundle does not vendor GitNexus."
            )
        # `npx -y <pkg>` auto-installs/caches the published package on first use.
        return [self.npx, "-y", self.package]


def detect_env(package: str = GITNEXUS_PACKAGE) -> GitNexusEnv:
    """Locate Node + npx without mutating anything."""
    return GitNexusEnv(
        node=shutil.which("node"),
        npx=shutil.which("npx"),
        package=package,
    )


def _run(
    cmd: list[str],
    cwd: str | os.PathLike[str] | None = None,
    timeout: int = 900,
    capture: bool = True,
) -> subprocess.CompletedProcess:
    try:
        return subprocess.run(
            cmd,
            cwd=str(cwd) if cwd else None,
            check=False,
            text=True,
            capture_output=capture,
            timeout=timeout,
        )
    except FileNotFoundError as exc:  # npx vanished mid-run
        raise GitNexusError(f"Failed to execute {cmd[0]!r}: {exc}") from exc
    except subprocess.TimeoutExpired as exc:
        raise GitNexusError(
            f"GitNexus command timed out after {timeout}s: {' '.join(cmd)}"
        ) from exc


@dataclass
class IndexState:
    """Whether a repo has a usable GitNexus index."""

    exists: bool
    path: Path
    age_seconds: float | None = None
    stale: bool = False
    details: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "exists": self.exists,
            "path": str(self.path),
            "age_seconds": self.age_seconds,
            "stale": self.stale,
            **({"details": self.details} if self.details else {}),
        }


def index_state(root: str = ".", stale_seconds: int = DEFAULT_STALE_SECONDS) -> IndexState:
    """Inspect the on-disk GitNexus index for ``root`` without running GitNexus."""
    index_path = Path(root).resolve() / INDEX_DIRNAME
    if not index_path.exists():
        return IndexState(exists=False, path=index_path)

    # Use the most recently modified file inside the index dir as the age basis.
    newest = 0.0
    for p in index_path.rglob("*"):
        if p.is_file():
            newest = max(newest, p.stat().st_mtime)
    age = time.time() - newest if newest else None
    stale = age is not None and age > stale_seconds
    return IndexState(exists=True, path=index_path, age_seconds=age, stale=stale)


def analyze(
    root: str = ".",
    env: GitNexusEnv | None = None,
    timeout: int = 1800,
) -> subprocess.CompletedProcess:
    """Build/refresh the GitNexus index for ``root`` (``gitnexus analyze``)."""
    env = env or detect_env()
    cmd = env.base_cmd() + ["analyze", "--path", str(Path(root).resolve())]
    proc = _run(cmd, cwd=root, timeout=timeout)
    if proc.returncode != 0:
        raise GitNexusError(
            f"`gitnexus analyze` failed (exit {proc.returncode}).\n{proc.stderr or proc.stdout}"
        )
    return proc


def ensure_index(
    root: str = ".",
    *,
    env: GitNexusEnv | None = None,
    stale_seconds: int = DEFAULT_STALE_SECONDS,
    auto: bool = True,
) -> IndexState:
    """Guarantee a fresh index exists.

    With ``auto=True`` (the bundle default) a missing or stale index is rebuilt
    automatically so agents always have graph context. With ``auto=False`` the
    caller is told to index but nothing is mutated.
    """
    env = env or detect_env()
    if not env.available:
        raise GitNexusError(
            "GitNexus is required but Node/npx is not available. "
            "Install Node.js (>=18) so coder agents are not flying blind."
        )
    state = index_state(root, stale_seconds=stale_seconds)
    if state.exists and not state.stale:
        return state
    if not auto:
        return state
    analyze(root, env=env)
    return index_state(root, stale_seconds=stale_seconds)


def _query(
    subcommand: list[str],
    root: str = ".",
    env: GitNexusEnv | None = None,
    timeout: int = 300,
) -> str:
    """Run a read-only GitNexus query command and return stdout."""
    env = env or detect_env()
    cmd = env.base_cmd() + subcommand
    proc = _run(cmd, cwd=root, timeout=timeout)
    if proc.returncode != 0:
        raise GitNexusError(
            f"`gitnexus {' '.join(subcommand)}` failed (exit {proc.returncode}).\n"
            f"{proc.stderr or proc.stdout}"
        )
    return proc.stdout.strip()


def ai_context(task: str, root: str = ".", env: GitNexusEnv | None = None) -> str:
    """Get task-scoped, graph-aware context for an agent (``gitnexus ai-context``)."""
    return _query(["ai-context", task], root=root, env=env)


def query(question: str, root: str = ".", env: GitNexusEnv | None = None) -> str:
    """Natural-language graph query (``gitnexus query``)."""
    return _query(["query", question], root=root, env=env)


def context(symbol: str, root: str = ".", env: GitNexusEnv | None = None) -> str:
    """Structural context for a symbol (``gitnexus context``)."""
    return _query(["context", symbol], root=root, env=env)


def impact(symbol: str, root: str = ".", env: GitNexusEnv | None = None) -> str:
    """Blast-radius / impact analysis for a symbol (``gitnexus impact``)."""
    return _query(["impact", symbol], root=root, env=env)


def doctor(root: str = ".", env: GitNexusEnv | None = None) -> dict[str, Any]:
    """Aggregate health report: runtime + index availability."""
    env = env or detect_env()
    report: dict[str, Any] = {
        "node": env.node,
        "npx": env.npx,
        "package": env.package,
        "available": env.available,
    }
    if env.available:
        state = index_state(root)
        report["index"] = state.to_dict()
    else:
        report["error"] = "Node.js/npx not found on PATH."
    return report


# --------------------------------------------------------------------------- #
# MCP wiring for coder agents
# --------------------------------------------------------------------------- #

# The single MCP server entry every agent should run. GitNexus exposes its graph
# tools over stdio via `gitnexus mcp`.
def mcp_server_command(package: str = GITNEXUS_PACKAGE) -> dict[str, Any]:
    return {"command": "npx", "args": ["-y", package, "mcp"]}


def _opencode_config_path() -> Path:
    return Path.home() / ".config" / "opencode" / "opencode.json"


def _codex_config_path() -> Path:
    return Path.home() / ".codex" / "config.toml"


def _hermes_config_path() -> Path:
    return Path.home() / ".hermes" / "mcp.json"


AGENTS = ("opencode", "codex", "hermes")


def _wire_opencode(package: str) -> str:
    path = _opencode_config_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    data: dict[str, Any] = {}
    if path.is_file():
        try:
            data = json.loads(path.read_text() or "{}")
        except json.JSONDecodeError:
            data = {}
    mcp = data.setdefault("mcp", {})
    mcp["gitnexus"] = {
        "type": "local",
        "command": ["npx", "-y", package, "mcp"],
        "enabled": True,
    }
    path.write_text(json.dumps(data, indent=2) + "\n")
    return str(path)


def _wire_codex(package: str) -> str:
    path = _codex_config_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    block = (
        "\n[mcp_servers.gitnexus]\n"
        'command = "npx"\n'
        f'args = ["-y", "{package}", "mcp"]\n'
    )
    existing = path.read_text() if path.is_file() else ""
    if "[mcp_servers.gitnexus]" in existing:
        return str(path)  # already wired; leave user edits intact
    path.write_text(existing + block)
    return str(path)


def _wire_hermes(package: str) -> str:
    path = _hermes_config_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    data: dict[str, Any] = {}
    if path.is_file():
        try:
            data = json.loads(path.read_text() or "{}")
        except json.JSONDecodeError:
            data = {}
    servers = data.setdefault("mcpServers", {})
    servers["gitnexus"] = mcp_server_command(package)
    path.write_text(json.dumps(data, indent=2) + "\n")
    return str(path)


_WIRERS = {
    "opencode": _wire_opencode,
    "codex": _wire_codex,
    "hermes": _wire_hermes,
}


def setup_agents(
    agents: list[str] | None = None,
    package: str = GITNEXUS_PACKAGE,
) -> dict[str, str]:
    """Wire the GitNexus MCP server into each agent's config.

    Returns a mapping of agent -> config file written.
    """
    chosen = agents or list(AGENTS)
    written: dict[str, str] = {}
    for agent in chosen:
        wirer = _WIRERS.get(agent)
        if not wirer:
            raise GitNexusError(f"Unknown agent: {agent!r}. Known: {', '.join(AGENTS)}")
        written[agent] = wirer(package)
    return written
