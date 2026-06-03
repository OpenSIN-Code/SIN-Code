# SPDX-License-Identifier: MIT
"""Generatoren fuer MCP-Client-Konfigurationen (WS2, Issue #2).

Erzeugt fertig einfuegbare Konfiguration fuer die drei Ziel-CLIs:

- ``opencode`` -> JSON  (Key ``mcp``, ``type: "local"``)
- ``codex``    -> TOML  (``[mcp_servers.sin]``)
- ``hermes``   -> YAML  (``mcp_servers.sin``)

Die Funktionen liefern reine Strings (fuer ``--stdout``) sowie Helfer zum
idempotenten Mergen in eine bestehende Konfigurationsdatei (fuer ``--write``).

Docs: mcp_config.doc.md
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

SERVER_NAME = "sin"
COMMAND = "sin"
ARGS = ["serve"]

# Standard-Env, das alle Clients durchreichen. Werte sind Platzhalter, die der
# Nutzer bei Bedarf anpasst; leere Defaults halten die Konfiguration gueltig.
DEFAULT_ENV: dict[str, str] = {}

SUPPORTED_CLIENTS = ("opencode", "codex", "hermes")

# All 15 individual SIN-Code tools (BR-3 / Issue #16).
# First 7 are Go binaries, remaining 8 are Python modules with MCP servers.
FULL_TOOLS: list[tuple[str, list[str]]] = [
    ("sin-discover", ["~/.local/bin/discover", "--mcp"]),
    ("sin-execute", ["~/.local/bin/execute", "--mcp"]),
    ("sin-map", ["~/.local/bin/map", "--mcp"]),
    ("sin-grasp", ["~/.local/bin/grasp", "--mcp"]),
    ("sin-scout", ["~/.local/bin/scout", "--mcp"]),
    ("sin-harvest", ["~/.local/bin/harvest", "--mcp"]),
    ("sin-orchestrate", ["~/.local/bin/orchestrate", "--mcp"]),
    ("sin-sckg", ["python", "-m", "sin_code_sckg.mcp_server"]),
    ("sin-ibd", ["python", "-m", "sin_code_ibd.mcp_server"]),
    ("sin-poc", ["python", "-m", "sin_code_poc.mcp_server"]),
    ("sin-efsm", ["python", "-m", "sin_code_efsm.mcp_server"]),
    ("sin-adw", ["python", "-m", "sin_code_adw.mcp_server"]),
    ("sin-oracle", ["python", "-m", "sin_code_oracle.mcp_server"]),
    ("sin-orchestration", ["python", "-m", "sin_code_orchestration.mcp_server"]),
    ("sin-review-interface", ["python", "-m", "sin_code_review_interface.mcp_server"]),
    ("sin-brain", ["python", "-m", "sin_brain.mcp_server"]),
]


# --------------------------------------------------------------------------- #
# Generatoren (reine Strings)
# --------------------------------------------------------------------------- #
def generate_opencode(env: dict[str, str] | None = None) -> str:
    """OpenCode liest ``opencode.json``: Key ``mcp`` mit lokalem Server.

    Format (offiziell dokumentiert):
        {
          "mcp": {
            "sin": {
              "type": "local",
              "command": ["sin", "serve"],
              "enabled": true,
              "environment": { ... }
            }
          }
        }
    """
    env = DEFAULT_ENV if env is None else env
    config = {
        "mcp": {
            SERVER_NAME: {
                "type": "local",
                "command": [COMMAND, *ARGS],
                "enabled": True,
                "environment": env,
            }
        }
    }
    return json.dumps(config, indent=2)


def generate_codex(env: dict[str, str] | None = None) -> str:
    """Codex liest ``~/.codex/config.toml``: ``[mcp_servers.<name>]``.

    Format (offiziell dokumentiert):
        [mcp_servers.sin]
        command = "sin"
        args = ["serve"]

        [mcp_servers.sin.env]
        KEY = "value"
    """
    env = DEFAULT_ENV if env is None else env
    lines = [
        f"[mcp_servers.{SERVER_NAME}]",
        f'command = "{COMMAND}"',
        f"args = {_toml_array(ARGS)}",
    ]
    if env:
        lines.append("")
        lines.append(f"[mcp_servers.{SERVER_NAME}.env]")
        for key, value in env.items():
            lines.append(f'{key} = "{value}"')
    return "\n".join(lines) + "\n"


def generate_hermes(env: dict[str, str] | None = None) -> str:
    """Hermes liest YAML: ``mcp_servers.<name>`` mit command/args.

    Format:
        mcp_servers:
          sin:
            command: sin
            args:
              - serve
            env: { ... }
    """
    env = DEFAULT_ENV if env is None else env
    server: dict[str, Any] = {
        "command": COMMAND,
        "args": list(ARGS),
    }
    if env:
        server["env"] = env
    config = {"mcp_servers": {SERVER_NAME: server}}
    try:
        import yaml

        return yaml.safe_dump(config, sort_keys=False, default_flow_style=False)
    except ImportError:
        # Fallback ohne PyYAML: minimaler, gueltiger YAML-Text.
        out = ["mcp_servers:", f"  {SERVER_NAME}:", f"    command: {COMMAND}", "    args:"]
        out += [f"      - {a}" for a in ARGS]
        if env:
            out.append("    env:")
            out += [f"      {k}: {v}" for k, v in env.items()]
        return "\n".join(out) + "\n"


def generate_full_opencode(env: dict[str, str] | None = None) -> str:
    """Full OpenCode config with all 15 individual SIN-Code tools (BR-3)."""
    env = DEFAULT_ENV if env is None else env
    mcp: dict[str, Any] = {}
    for name, cmd in FULL_TOOLS:
        mcp[name] = {
            "type": "local",
            "command": list(cmd),
            "enabled": True,
            "environment": env,
        }
    return json.dumps({"mcp": mcp}, indent=2)


def generate_full_codex(env: dict[str, str] | None = None) -> str:
    """Full Codex TOML config with all 15 individual SIN-Code tools (BR-3)."""
    env = DEFAULT_ENV if env is None else env
    blocks: list[str] = []
    for name, cmd in FULL_TOOLS:
        lines = [
            f"[mcp_servers.{name}]",
            f'command = "{cmd[0]}"',
            f"args = {_toml_array(cmd[1:])}",
        ]
        if env:
            lines.append("")
            lines.append(f"[mcp_servers.{name}.env]")
            for key, value in env.items():
                lines.append(f'{key} = "{value}"')
        blocks.append("\n".join(lines))
    return "\n\n".join(blocks) + "\n"


def generate_full_hermes(env: dict[str, str] | None = None) -> str:
    """Full Hermes YAML config with all 15 individual SIN-Code tools (BR-3)."""
    env = DEFAULT_ENV if env is None else env
    servers: dict[str, Any] = {}
    for name, cmd in FULL_TOOLS:
        server = {"command": cmd[0], "args": list(cmd[1:])}
        if env:
            server["env"] = env
        servers[name] = server
    config = {"mcp_servers": servers}
    try:
        import yaml

        return yaml.safe_dump(config, sort_keys=False, default_flow_style=False)
    except ImportError:
        out = ["mcp_servers:"]
        for name, cmd in FULL_TOOLS:
            out.append(f"  {name}:")
            out.append(f"    command: {cmd[0]}")
            out.append("    args:")
            out += [f"      - {a}" for a in cmd[1:]]
            if env:
                out.append("    env:")
                out += [f"      {k}: {v}" for k, v in env.items()]
        return "\n".join(out) + "\n"


def generate_full(client: str, env: dict[str, str] | None = None) -> str:
    """Dispatch full config nach Client-Name."""
    client = client.lower()
    if client == "opencode":
        return generate_full_opencode(env)
    if client == "codex":
        return generate_full_codex(env)
    if client == "hermes":
        return generate_full_hermes(env)
    raise ValueError(
        f"Unknown client '{client}'. Supported: {', '.join(SUPPORTED_CLIENTS)}"
    )


def generate(client: str, env: dict[str, str] | None = None) -> str:
    """Dispatch nach Client-Name."""
    client = client.lower()
    if client == "opencode":
        return generate_opencode(env)
    if client == "codex":
        return generate_codex(env)
    if client == "hermes":
        return generate_hermes(env)
    raise ValueError(
        f"Unknown client '{client}'. Supported: {', '.join(SUPPORTED_CLIENTS)}"
    )


# --------------------------------------------------------------------------- #
# Default-Zielpfade pro Client
# --------------------------------------------------------------------------- #
def default_path(client: str) -> Path:
    """Konventioneller Konfigurationspfad des jeweiligen Clients."""
    client = client.lower()
    if client == "opencode":
        return Path("opencode.json")
    if client == "codex":
        return Path.home() / ".codex" / "config.toml"
    if client == "hermes":
        return Path.home() / ".hermes" / "config.yaml"
    raise ValueError(f"Unknown client '{client}'")


# --------------------------------------------------------------------------- #
# Idempotentes Mergen in bestehende Dateien (--write)
# --------------------------------------------------------------------------- #
def merge_into_file(client: str, path: Path, env: dict[str, str] | None = None) -> str:
    """Fuegt den sin-Server in eine bestehende Config-Datei ein bzw. legt sie an.

    Gibt eine kurze Statusmeldung zurueck. Bestehende fremde Eintraege bleiben
    erhalten; ein vorhandener ``sin``-Eintrag wird ersetzt.
    """
    client = client.lower()
    if client == "opencode":
        return _merge_json(path, env)
    if client == "hermes":
        return _merge_yaml(path, env)
    if client == "codex":
        return _merge_codex_toml(path, env)
    raise ValueError(f"Unknown client '{client}'")


def merge_full_into_file(client: str, path: Path, env: dict[str, str] | None = None) -> str:
    """Fuegt alle 15 SIN-Code MCP-Server in eine bestehende Config-Datei ein (BR-3).

    Gibt eine kurze Statusmeldung zurueck. Bestehende fremde Eintraege bleiben
    erhalten; vorhandene ``sin-*``-Eintraege werden ersetzt.
    """
    client = client.lower()
    if client == "opencode":
        return _merge_json_full(path, env)
    if client == "hermes":
        return _merge_yaml_full(path, env)
    if client == "codex":
        return _merge_codex_toml_full(path, env)
    raise ValueError(f"Unknown client '{client}'")


def _merge_json(path: Path, env: dict[str, str] | None) -> str:
    data: dict[str, Any] = {}
    if path.exists() and path.read_text().strip():
        try:
            data = json.loads(path.read_text())
        except json.JSONDecodeError as exc:
            raise ValueError(f"Existing {path} is not valid JSON: {exc}") from exc
    mcp = data.setdefault("mcp", {})
    mcp[SERVER_NAME] = {
        "type": "local",
        "command": [COMMAND, *ARGS],
        "enabled": True,
        "environment": DEFAULT_ENV if env is None else env,
    }
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2) + "\n")
    return f"Merged 'sin' MCP server into {path}"


def _merge_yaml(path: Path, env: dict[str, str] | None) -> str:
    try:
        import yaml
    except ImportError as exc:  # pragma: no cover - pyyaml ist Pflicht-Dep
        raise ValueError("PyYAML required to merge YAML config") from exc

    data: dict[str, Any] = {}
    if path.exists() and path.read_text().strip():
        loaded = yaml.safe_load(path.read_text())
        if isinstance(loaded, dict):
            data = loaded
    servers = data.setdefault("mcp_servers", {})
    server: dict[str, Any] = {"command": COMMAND, "args": list(ARGS)}
    if env:
        server["env"] = env
    servers[SERVER_NAME] = server
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(yaml.safe_dump(data, sort_keys=False, default_flow_style=False))
    return f"Merged 'sin' MCP server into {path}"


def _merge_codex_toml(path: Path, env: dict[str, str] | None) -> str:
    """Merge fuer TOML ohne externe Writer-Dependency.

    Strategie: vorhandenen ``[mcp_servers.sin]``-Block (inkl. Sub-Table
    ``.env``) entfernen und den frisch generierten Block anhaengen. Andere
    Tabellen bleiben unangetastet.
    """
    existing = ""
    if path.exists():
        existing = path.read_text()
    cleaned = _strip_toml_table(existing, f"mcp_servers.{SERVER_NAME}")
    block = generate_codex(env)
    sep = "" if cleaned == "" or cleaned.endswith("\n\n") else ("\n" if cleaned.endswith("\n") else "\n\n")
    new_content = cleaned + sep + block
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(new_content)
    return f"Merged 'sin' MCP server into {path}"


# --------------------------------------------------------------------------- #
# Full-config merge helpers (BR-3)
# --------------------------------------------------------------------------- #
def _merge_json_full(path: Path, env: dict[str, str] | None) -> str:
    data: dict[str, Any] = {}
    if path.exists() and path.read_text().strip():
        try:
            data = json.loads(path.read_text())
        except json.JSONDecodeError as exc:
            raise ValueError(f"Existing {path} is not valid JSON: {exc}") from exc
    mcp = data.setdefault("mcp", {})
    for name, cmd in FULL_TOOLS:
        mcp[name] = {
            "type": "local",
            "command": list(cmd),
            "enabled": True,
            "environment": DEFAULT_ENV if env is None else env,
        }
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2) + "\n")
    return f"Merged {len(FULL_TOOLS)} MCP servers into {path}"


def _merge_yaml_full(path: Path, env: dict[str, str] | None) -> str:
    try:
        import yaml
    except ImportError as exc:  # pragma: no cover - pyyaml ist Pflicht-Dep
        raise ValueError("PyYAML required to merge YAML config") from exc

    data: dict[str, Any] = {}
    if path.exists() and path.read_text().strip():
        loaded = yaml.safe_load(path.read_text())
        if isinstance(loaded, dict):
            data = loaded
    servers = data.setdefault("mcp_servers", {})
    for name, cmd in FULL_TOOLS:
        server: dict[str, Any] = {"command": cmd[0], "args": list(cmd[1:])}
        if env:
            server["env"] = env
        servers[name] = server
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(yaml.safe_dump(data, sort_keys=False, default_flow_style=False))
    return f"Merged {len(FULL_TOOLS)} MCP servers into {path}"


def _merge_codex_toml_full(path: Path, env: dict[str, str] | None) -> str:
    """Merge fuer TOML mit allen 15 SIN-Code Tools.

    Strategie: vorhandene ``[mcp_servers.sin]`` und ``[mcp_servers.sin-*]``
    Bloecke entfernen und frisch generierte Bloecke anhaengen.
    """
    existing = ""
    if path.exists():
        existing = path.read_text()
    # Remove old single server and all full-tool tables
    existing = _strip_toml_table(existing, f"mcp_servers.{SERVER_NAME}")
    for name, _ in FULL_TOOLS:
        existing = _strip_toml_table(existing, f"mcp_servers.{name}")
    blocks: list[str] = []
    for name, cmd in FULL_TOOLS:
        lines = [
            f"[mcp_servers.{name}]",
            f'command = "{cmd[0]}"',
            f"args = {_toml_array(cmd[1:])}",
        ]
        if env:
            lines.append("")
            lines.append(f"[mcp_servers.{name}.env]")
            for key, value in env.items():
                lines.append(f'{key} = "{value}"')
        blocks.append("\n".join(lines))
    block = "\n\n".join(blocks) + "\n"
    sep = "" if existing == "" or existing.endswith("\n\n") else ("\n" if existing.endswith("\n") else "\n\n")
    new_content = existing + sep + block
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(new_content)
    return f"Merged {len(FULL_TOOLS)} MCP servers into {path}"


# --------------------------------------------------------------------------- #
# Hilfsfunktionen
# --------------------------------------------------------------------------- #
def _toml_array(items: list[str]) -> str:
    inner = ", ".join(f'"{i}"' for i in items)
    return f"[{inner}]"


def _strip_toml_table(content: str, table_prefix: str) -> str:
    """Entfernt ``[table_prefix]`` und alle Sub-Tables ``[table_prefix.*]``.

    Zeilenbasiert und bewusst simpel: ausreichend fuer das von uns erzeugte
    Format und fremde, klar getrennte Tabellen.
    """
    if not content:
        return ""
    lines = content.splitlines()
    out: list[str] = []
    skip = False
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("[") and stripped.endswith("]"):
            name = stripped[1:-1].strip()
            # Header-Form [[name]] reduziert sich nach obigem Slicing auf [name]
            name = name.lstrip("[").rstrip("]").strip()
            if name == table_prefix or name.startswith(table_prefix + "."):
                skip = True
                continue
            skip = False
        if not skip:
            out.append(line)
    # fuehrende/abschliessende Leerzeilen normalisieren
    text = "\n".join(out).strip("\n")
    return text + "\n" if text else ""
