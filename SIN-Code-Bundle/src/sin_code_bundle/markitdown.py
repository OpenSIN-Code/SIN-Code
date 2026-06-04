"""MarkItDown bridge.

MarkItDown (https://github.com/microsoft/markitdown) is an *upstream* tool by
Microsoft, distributed as the MIT-licensed PyPI packages ``markitdown`` (CLI /
library) and ``markitdown-mcp`` (an MCP server). We never vendor or copy its
source; we only invoke the published packages. This keeps the bundle
MIT-licensed while giving coder agents a first-class way to turn binary and
office documents (PDF, DOCX, PPTX, XLSX, images, audio, HTML, ...) into
LLM-friendly Markdown.

The bridge provides:
  * discovery / health checks for the ``markitdown-mcp`` runner and the
    ``markitdown`` CLI,
  * a thin ``convert`` wrapper over the ``markitdown`` CLI,
  * MCP wiring so OpenCode / Codex / Hermes each get the MarkItDown MCP server,
    mirroring upstream's recommended ``uvx markitdown-mcp`` invocation.
"""

from __future__ import annotations

import json
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Any

# MarkItDown exposes its MCP server through the ``markitdown-mcp`` package.
# Upstream recommends running it via ``uvx`` so it is fetched/cached on demand;
# we fall back to a directly-installed ``markitdown-mcp`` executable.
MARKITDOWN_MCP_PACKAGE = "markitdown-mcp"
MARKITDOWN_CLI = "markitdown"


class MarkItDownError(RuntimeError):
    """Raised when MarkItDown is unavailable or a command fails."""


@dataclass
class MarkItDownEnv:
    """Resolved runtime environment for invoking MarkItDown."""

    uvx: str | None
    mcp_exe: str | None
    cli: str | None

    @property
    def mcp_available(self) -> bool:
        return bool(self.uvx or self.mcp_exe)

    @property
    def cli_available(self) -> bool:
        return bool(self.cli)

    def mcp_command(self) -> dict[str, Any]:
        """Return the MCP launch command, preferring ``uvx``."""
        if self.uvx:
            return {"command": "uvx", "args": [MARKITDOWN_MCP_PACKAGE]}
        if self.mcp_exe:
            return {"command": MARKITDOWN_MCP_PACKAGE, "args": []}
        raise MarkItDownError(
            "MarkItDown MCP server not found. Install it with "
            "`pip install markitdown-mcp` (or `uv tool install markitdown-mcp`). "
            "The bundle does not vendor MarkItDown."
        )

    def cli_cmd(self) -> str:
        if not self.cli:
            raise MarkItDownError(
                "`markitdown` CLI not found. Install with `pip install 'markitdown[all]'`."
            )
        return self.cli


def detect_env() -> MarkItDownEnv:
    return MarkItDownEnv(
        uvx=shutil.which("uvx"),
        mcp_exe=shutil.which(MARKITDOWN_MCP_PACKAGE),
        cli=shutil.which(MARKITDOWN_CLI),
    )


def mcp_server_command(env: MarkItDownEnv | None = None) -> dict[str, Any]:
    """Resolve the MCP server launch command (``uvx markitdown-mcp`` by default)."""
    env = env or detect_env()
    return env.mcp_command()


def convert(path: str, env: MarkItDownEnv | None = None, timeout: int = 300) -> str:
    """Convert a document to Markdown using the upstream ``markitdown`` CLI."""
    env = env or detect_env()
    cli = env.cli_cmd()
    src = Path(path)
    if not src.is_file():
        raise MarkItDownError(f"File not found: {path}")
    try:
        proc = subprocess.run(
            [cli, str(src)],
            capture_output=True,
            text=True,
            timeout=timeout,
        )
    except subprocess.TimeoutExpired as exc:  # pragma: no cover - timing dependent
        raise MarkItDownError(f"markitdown timed out after {timeout}s") from exc
    if proc.returncode != 0:
        raise MarkItDownError(f"markitdown failed ({proc.returncode}): {proc.stderr.strip()}")
    return proc.stdout


def doctor() -> dict[str, Any]:
    """Report MarkItDown availability for diagnostics."""
    env = detect_env()
    return {
        "mcp_available": env.mcp_available,
        "cli_available": env.cli_available,
        "runner": "uvx" if env.uvx else (MARKITDOWN_MCP_PACKAGE if env.mcp_exe else None),
        "mcp_package": MARKITDOWN_MCP_PACKAGE,
    }


# --------------------------------------------------------------------------- #
# MCP wiring into coder-agent configs (mirrors the GitNexus bridge).
# --------------------------------------------------------------------------- #
def _opencode_config_path() -> Path:
    return Path.home() / ".config" / "opencode" / "opencode.json"


def _codex_config_path() -> Path:
    return Path.home() / ".codex" / "config.toml"


def _hermes_config_path() -> Path:
    return Path.home() / ".hermes" / "mcp.json"


AGENTS = ("opencode", "codex", "hermes")


def _launch(env: MarkItDownEnv | None) -> tuple[str, list[str]]:
    cmd = mcp_server_command(env)
    return cmd["command"], cmd["args"]


def _wire_opencode(env: MarkItDownEnv | None) -> str:
    command, args = _launch(env)
    path = _opencode_config_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    data: dict[str, Any] = {}
    if path.is_file():
        try:
            data = json.loads(path.read_text() or "{}")
        except json.JSONDecodeError:
            data = {}
    mcp = data.setdefault("mcp", {})
    mcp["markitdown"] = {
        "type": "local",
        "command": [command, *args],
        "enabled": True,
    }
    path.write_text(json.dumps(data, indent=2) + "\n")
    return str(path)


def _wire_codex(env: MarkItDownEnv | None) -> str:
    command, args = _launch(env)
    path = _codex_config_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    args_repr = ", ".join(f'"{a}"' for a in args)
    block = f'\n[mcp_servers.markitdown]\ncommand = "{command}"\nargs = [{args_repr}]\n'
    existing = path.read_text() if path.is_file() else ""
    if "[mcp_servers.markitdown]" in existing:
        return str(path)  # already wired; leave user edits intact
    path.write_text(existing + block)
    return str(path)


def _wire_hermes(env: MarkItDownEnv | None) -> str:
    command, args = _launch(env)
    path = _hermes_config_path()
    path.parent.mkdir(parents=True, exist_ok=True)
    data: dict[str, Any] = {}
    if path.is_file():
        try:
            data = json.loads(path.read_text() or "{}")
        except json.JSONDecodeError:
            data = {}
    servers = data.setdefault("mcpServers", {})
    servers["markitdown"] = {"command": command, "args": args}
    path.write_text(json.dumps(data, indent=2) + "\n")
    return str(path)


_WIRERS = {
    "opencode": _wire_opencode,
    "codex": _wire_codex,
    "hermes": _wire_hermes,
}


def setup_agents(
    agents: list[str] | None = None,
    env: MarkItDownEnv | None = None,
) -> dict[str, str]:
    """Wire the MarkItDown MCP server into each agent's config.

    Returns a mapping of agent -> config file written.
    """
    chosen = agents or list(AGENTS)
    written: dict[str, str] = {}
    for agent in chosen:
        wirer = _WIRERS.get(agent)
        if not wirer:
            raise MarkItDownError(f"Unknown agent: {agent!r}. Known: {', '.join(AGENTS)}")
        written[agent] = wirer(env)
    return written
