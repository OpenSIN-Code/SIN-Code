"""Config + AGENTS.md generators for the SIN-Code Bundle.

These power ``sin init <agent>`` and ``sin agents-md``. The goal: any supported
coding agent can discover and best-practice-use the SIN-Code MCP server with a
single command.

Supported agents:
- opencode  -> opencode.json               ({"mcp": {...}})
- codex     -> .codex/config.toml          ([mcp_servers.sin])
- hermes    -> .hermes/config.yaml         (mcp_servers: {...})
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Literal

try:  # pyyaml is already a hard dependency of the bundle
    import yaml
except ImportError:  # pragma: no cover
    yaml = None  # type: ignore[assignment]

Agent = Literal["opencode", "codex", "hermes"]
Scope = Literal["local", "global"]

SERVER_NAME = "sin"
SERVER_COMMAND = "sin"
SERVER_ARGS = ["serve"]

SUPPORTED_AGENTS: tuple[Agent, ...] = ("opencode", "codex", "hermes")

# --------------------------------------------------------------------------- #
# AGENTS.md content
# --------------------------------------------------------------------------- #
AGENTS_MD = """\
# AGENTS.md — SIN-Code Engineering Doctrine

> Binding for every AI coding agent working in this repository. The closest
> AGENTS.md to an edited file wins; explicit user instructions override it.

## Tools available via the SIN-Code MCP server (`sin serve`)

| Tool | Use it to … | Call it … |
|------|-------------|-----------|
| `impact(symbol_fqid)` | See the blast radius of a symbol | BEFORE editing |
| `semantic_diff(file_a, file_b)` | Intent + risk of a change | AFTER editing |
| `semantic_review(file_a, file_b)` | Intent + risk + recommendation | AFTER editing |
| `architectural_debt()` | Current complexity/debt score | BEFORE + AFTER |
| `prove(function_code, properties)` | Proof of correctness | risky pure logic |
| `verify_tests(code, language)` | Independent verification | before "done" |
| `mock_env(action, port)` | Ephemeral full-stack mock | integration work |

## The non-negotiable loop

1. **Orient** — if `.sin/` is missing, run `sin bootstrap .`.
2. **Assess impact** — `impact(<fqid>)` before editing a symbol.
3. **Edit minimally.**
4. **Review** — `semantic_review(before, after)`; if risk != low, justify it.
5. **Guard debt** — `architectural_debt()`; respect the ADW breaker.
6. **Verify** — `verify_tests` (+ `prove` for critical functions).
   **Do NOT report done while verification is red.**

## Hard rules

- Never claim "done" without a green verification.
- Never bypass the ADW cost/complexity breaker.
- Prefer `impact` over grep for "what calls this?".
- One concern per change; split multi-intent diffs.

## Dev / test

- Python 3.11+; install `pip install -e ".[mcp]"`; `sin bootstrap .` once.
- `pytest -q` must be green AND `verify_tests` must return `pass` before done.
- PR title format: `[sin-code-bundle] <Title>`.
"""


def render_agents_md() -> str:
    """Return the canonical AGENTS.md content."""
    return AGENTS_MD


# --------------------------------------------------------------------------- #
# Agent config targets
# --------------------------------------------------------------------------- #
@dataclass(frozen=True)
class ConfigTarget:
    """Where an agent expects its config and how it is serialised."""

    agent: Agent
    fmt: Literal["json", "toml", "yaml"]
    local_path: Path
    global_path: Path

    def path(self, scope: Scope) -> Path:
        return self.local_path if scope == "local" else self.global_path


def _targets() -> dict[Agent, ConfigTarget]:
    home = Path.home()
    return {
        "opencode": ConfigTarget(
            agent="opencode",
            fmt="json",
            local_path=Path("opencode.json"),
            global_path=home / ".config" / "opencode" / "opencode.json",
        ),
        "codex": ConfigTarget(
            agent="codex",
            fmt="toml",
            local_path=Path(".codex") / "config.toml",
            global_path=home / ".codex" / "config.toml",
        ),
        "hermes": ConfigTarget(
            agent="hermes",
            fmt="yaml",
            local_path=Path(".hermes") / "config.yaml",
            global_path=home / ".hermes" / "config.yaml",
        ),
    }


# --------------------------------------------------------------------------- #
# Idempotent mergers
# --------------------------------------------------------------------------- #
def _merge_opencode(existing: dict) -> dict:
    existing.setdefault("$schema", "https://opencode.ai/config.json")
    mcp = existing.setdefault("mcp", {})
    mcp[SERVER_NAME] = {
        "type": "local",
        "command": [SERVER_COMMAND, *SERVER_ARGS],
        "enabled": True,
    }
    return existing


def _merge_codex(existing: dict) -> dict:
    servers = existing.setdefault("mcp_servers", {})
    servers[SERVER_NAME] = {
        "command": SERVER_COMMAND,
        "args": list(SERVER_ARGS),
    }
    return existing


def _merge_hermes(existing: dict) -> dict:
    servers = existing.setdefault("mcp_servers", {})
    servers[SERVER_NAME] = {
        "command": SERVER_COMMAND,
        "args": list(SERVER_ARGS),
    }
    return existing


_MERGERS: dict[str, object] = {
    "opencode": _merge_opencode,
    "codex": _merge_codex,
    "hermes": _merge_hermes,
}


# --------------------------------------------------------------------------- #
# (De)serialisation
# --------------------------------------------------------------------------- #
def _load(path: Path, fmt: str) -> dict:
    if not path.exists() or path.stat().st_size == 0:
        return {}
    text = path.read_text(encoding="utf-8")
    if fmt == "json":
        return json.loads(text)
    if fmt == "yaml":
        if yaml is None:
            raise RuntimeError("pyyaml is required for hermes config")
        return yaml.safe_load(text) or {}
    if fmt == "toml":
        import tomllib  # stdlib on Python 3.11+

        return tomllib.loads(text)
    raise ValueError(f"unknown format: {fmt}")


def _dump(data: dict, fmt: str) -> str:
    if fmt == "json":
        return json.dumps(data, indent=2) + "\n"
    if fmt == "yaml":
        if yaml is None:
            raise RuntimeError("pyyaml is required for hermes config")
        return yaml.safe_dump(data, sort_keys=False)
    if fmt == "toml":
        return _dump_toml(data)
    raise ValueError(f"unknown format: {fmt}")


def _dump_toml(data: dict) -> str:
    """Minimal, dependency-free TOML writer for the [mcp_servers.*] shape.

    Avoids requiring ``tomli-w``. Only handles the structures we actually emit:
    str, list[str], and nested [mcp_servers.<name>] tables.
    """

    def fmt_value(value: object) -> str:
        if isinstance(value, bool):
            return "true" if value else "false"
        if isinstance(value, str):
            return json.dumps(value)
        if isinstance(value, list):
            return "[" + ", ".join(fmt_value(v) for v in value) + "]"
        return json.dumps(value)

    lines: list[str] = []
    for name, cfg in data.get("mcp_servers", {}).items():
        lines.append(f"[mcp_servers.{name}]")
        for key, value in cfg.items():
            lines.append(f"{key} = {fmt_value(value)}")
        lines.append("")
    return ("\n".join(lines)).strip() + "\n"


# --------------------------------------------------------------------------- #
# Public API
# --------------------------------------------------------------------------- #
def render_agent_config(agent: Agent, scope: Scope = "local") -> tuple[Path, str]:
    """Return ``(target_path, rendered_content)`` for an agent, merging existing."""
    targets = _targets()
    if agent not in targets:
        raise ValueError(f"unknown agent '{agent}'. Supported: {', '.join(targets)}")
    target = targets[agent]
    path = target.path(scope)
    existing = _load(path, target.fmt)
    merged = _MERGERS[agent](existing)  # type: ignore[operator]
    return path, _dump(merged, target.fmt)


def write_agent_config(
    agent: Agent,
    scope: Scope = "local",
    dry_run: bool = False,
) -> tuple[Path, str]:
    """Write (or preview) the agent config. Returns ``(path, content)``."""
    path, content = render_agent_config(agent, scope)
    if not dry_run:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
    return path, content


def write_agents_md(
    root: Path = Path("."),
    dry_run: bool = False,
    force: bool = False,
) -> tuple[Path, bool]:
    """Write AGENTS.md to *root*. Returns ``(path, was_written)``."""
    path = root / "AGENTS.md"
    if path.exists() and not force:
        return path, False
    if not dry_run:
        path.write_text(render_agents_md(), encoding="utf-8")
    return path, not dry_run
