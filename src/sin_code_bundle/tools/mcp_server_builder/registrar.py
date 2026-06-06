"""
Purpose: Register a new MCP server in opencode.json (mcp section).
Docs: registrar.doc.md
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass, field
from pathlib import Path

# ── Constants ──────────────────────────────────────
# Search order for opencode.json: explicit path > $OPENCODE_CONFIG > ~/.config/opencode/opencode.json
DEFAULT_OPENCODE_PATHS = [
    Path.home() / ".config" / "opencode" / "opencode.json",
    Path.home() / ".config" / "opencode" / "opencode.jsonc",
]


@dataclass
class McpServerEntry:
    """Lightweight representation of an MCP server entry in opencode.json."""

    name: str
    type: str  # "local" or "remote"
    command: list[str] = field(default_factory=list)
    url: str = ""
    enabled: bool = True
    env: dict[str, str] = field(default_factory=dict)
    headers: dict[str, str] = field(default_factory=dict)

    def to_json(self) -> dict:
        """Serialize for the opencode.json mcp section."""
        entry: dict = {"type": self.type, "enabled": self.enabled}
        if self.command:
            entry["command"] = self.command
        if self.url:
            entry["url"] = self.url
        if self.env:
            entry["env"] = self.env
        if self.headers:
            entry["headers"] = self.headers
        return entry


@dataclass
class Registrar:
    """Register/unregister MCP servers in an opencode.json file.

    Pure stdlib — preserves `$schema`, key order, and unknown fields.
    """

    config_path: Path | None = None

    def __post_init__(self) -> None:
        if self.config_path is None:
            self.config_path = self._discover()

    def _discover(self) -> Path:
        """Find the active opencode.json or fall back to the default location."""
        env = os.getenv("OPENCODE_CONFIG")
        if env:
            p = Path(env).expanduser()
            return p
        for cand in DEFAULT_OPENCODE_PATHS:
            if cand.is_file():
                return cand
        # Default — even if missing, we'll create it.
        return DEFAULT_OPENCODE_PATHS[0]

    def register(self, entry: McpServerEntry) -> Path:
        """Insert or replace `entry` under `mcp.<name>`. Returns the config path."""
        path = self._ensure_path()
        data = self._load(path)
        data.setdefault("mcp", {})
        data["mcp"][entry.name] = entry.to_json()
        self._save(path, data)
        return path

    def unregister(self, name: str) -> bool:
        """Remove `mcp.<name>` if present. Returns True if removed."""
        path = self.config_path or DEFAULT_OPENCODE_PATHS[0]
        if not path.is_file():
            return False
        data = self._load(path)
        mcp = data.get("mcp", {})
        if name in mcp:
            del mcp[name]
            self._save(path, data)
            return True
        return False

    def list_registered(self) -> list[str]:
        """Return all currently registered MCP server names."""
        if not (self.config_path and self.config_path.is_file()):
            return []
        data = self._load(self.config_path)
        return sorted((data.get("mcp") or {}).keys())

    def show(self, name: str) -> dict | None:
        """Return the mcp.<name> entry as a dict, or None if missing."""
        if not (self.config_path and self.config_path.is_file()):
            return None
        data = self._load(self.config_path)
        return (data.get("mcp") or {}).get(name)

    # ── Internals ──────────────────────────────────────

    def _ensure_path(self) -> Path:
        """Create the parent directory of `self.config_path` if missing."""
        assert self.config_path is not None
        self.config_path.parent.mkdir(parents=True, exist_ok=True)
        return self.config_path

    def _load(self, path: Path) -> dict:
        """Load JSON from `path`. Returns `{}` on missing/empty file."""
        if not path.is_file():
            return {}
        text = path.read_text(encoding="utf-8").strip()
        if not text:
            return {}
        # Tolerate JSONC trailing commas by stripping them.
        text = self._strip_jsonc(text)
        return json.loads(text)

    def _save(self, path: Path, data: dict) -> None:
        """Write JSON with stable indentation; preserves `$schema` if present."""
        path.write_text(
            json.dumps(data, indent=2, sort_keys=False) + "\n",
            encoding="utf-8",
        )

    @staticmethod
    def _strip_jsonc(text: str) -> str:
        """Naive JSONC cleanup — strip line/block comments and trailing commas.

        Only strips `//` comments that are NOT inside a string. The simple
        implementation works for the well-formed JSONC emitted by opencode.
        """
        import re

        out: list[str] = []
        i = 0
        in_string = False
        escape = False
        while i < len(text):
            ch = text[i]
            if in_string:
                out.append(ch)
                if escape:
                    escape = False
                elif ch == "\\":
                    escape = True
                elif ch == '"':
                    in_string = False
                i += 1
                continue
            if ch == '"':
                in_string = True
                out.append(ch)
                i += 1
                continue
            # Outside string — strip // line comments and /* block */ comments.
            if ch == "/" and i + 1 < len(text) and text[i + 1] == "/":
                # Skip to end of line.
                while i < len(text) and text[i] != "\n":
                    i += 1
                continue
            if ch == "/" and i + 1 < len(text) and text[i + 1] == "*":
                i += 2
                while i + 1 < len(text) and not (text[i] == "*" and text[i + 1] == "/"):
                    i += 1
                i += 2  # past the */
                continue
            out.append(ch)
            i += 1
        text = "".join(out)
        text = re.sub(r",\s*([\]}])", r"\1", text)
        return text


def build_local_entry(
    name: str, command: list[str], env: dict | None = None
) -> McpServerEntry:
    """Helper: build a `type: local` entry with `command` as a list of strings."""
    return McpServerEntry(
        name=name,
        type="local",
        command=list(command),
        env=dict(env or {}),
    )
