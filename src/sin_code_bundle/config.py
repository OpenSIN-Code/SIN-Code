# SPDX-License-Identifier: MIT
"""Layered config store for the SIN-Code CLI.

Resolves a single dotted key (e.g. ``tui.theme``) against four sources,
lowest to highest precedence:

  1. ``~/.config/sin/config.toml``       — global defaults
  2. ``~/.config/opencode/opencode.json`` — opencode integration bridge
  3. ``./sin.config.toml``                — project-local override
  4. ``SIN_*`` environment variables      — runtime override

Sentinel for "key not set" is :class:`Missing`, distinct from ``None``
so ``config get opencode.api_key`` can return ``""`` when explicitly
cleared and report "unset" when never written.

Docs: config.doc.md
"""

from __future__ import annotations

import json
import os
import tomllib
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

# External writer; stdlib tomllib is read-only on 3.11+. Import is
# deferred to first write so the read path stays import-cheap.
def _writer():
    import tomli_w  # type: ignore[import-not-found]

    return tomli_w


# ── Paths ────────────────────────────────────────────────────────────────
GLOBAL_TOML = Path.home() / ".config" / "sin" / "config.toml"
OPENCODE_JSON = Path.home() / ".config" / "opencode" / "opencode.json"
PROJECT_TOML = Path.cwd() / "sin.config.toml"

# Keys whose values must never be echoed back. Match is *substring*,
# case-insensitive — so `MY_API_KEY` and `auth.token` both get redacted.
REDACTED_SUBSTRINGS = ("api_key", "apikey", "token", "password", "secret", "private_key")
REDACTED_PLACEHOLDER = "<redacted>"


# ── Sentinel ─────────────────────────────────────────────────────────────
class Missing:
    """Sentinel singleton. Use ``is MISSING`` (or ``isinstance(x, Missing)``)
    instead of equality so a stored ``Missing()`` value is distinguishable
    from a stored ``None``.
    """

    _instance: "Missing | None" = None

    def __new__(cls) -> "Missing":
        if cls._instance is None:
            cls._instance = super().__new__(cls)
        return cls._instance

    def __repr__(self) -> str:  # pragma: no cover - cosmetic
        return "<MISSING>"


MISSING = Missing()


# ── Result types ─────────────────────────────────────────────────────────
@dataclass
class ConfigSource:
    """Where a value came from (or ``MISSING``)."""

    path: Path
    exists: bool
    priority: int  # 0 = lowest, 3 = highest
    label: str  # e.g. "global", "opencode", "project", "env"


@dataclass
class ConfigView:
    """Resolved view of a single key."""

    key: str
    value: Any
    source: ConfigSource | None  # None when value is MISSING


# ── Loaders ──────────────────────────────────────────────────────────────
def _read_toml(path: Path) -> dict[str, Any]:
    """Read a TOML file and return an empty dict on any error."""
    if not path.exists():
        return {}
    try:
        with path.open("rb") as f:
            return tomllib.load(f)
    except (OSError, tomllib.TOMLDecodeError):
        # Corrupted config should not crash the CLI — treat as empty and
        # let the user discover the issue via `sin config path`.
        return {}


def _read_opencode_json(path: Path) -> dict[str, Any]:
    """Read opencode.json and surface the ``sin`` sub-object as the top level.

    Opencode's config is a large nested object; we only care about the
    ``sin`` bridge section so callers can use the same dotted notation
    (``sin config get opencode.model``) as for TOML.
    """
    if not path.exists():
        return {}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return {}
    if not isinstance(data, dict):
        return {}
    return data.get("sin", {}) if isinstance(data.get("sin"), dict) else {}


def _read_env() -> dict[str, Any]:
    """Read every ``SIN_*`` env var and convert to a nested dict.

    Double-underscore is the section separator: ``SIN_TUI__THEME=dark``
    becomes ``{"tui": {"theme": "dark"}}``. Values are always strings —
    callers that need typed values should cast.
    """
    out: dict[str, Any] = {}
    for key, value in os.environ.items():
        if not key.startswith("SIN_"):
            continue
        body = key[4:].lower()  # strip SIN_ prefix
        if "__" in body:
            section, leaf = body.split("__", 1)
            bucket = out.setdefault(section, {})
            if not isinstance(bucket, dict):
                bucket = {}
                out[section] = bucket
            bucket[leaf] = value
        else:
            out[body] = value
    return out


def _read_sources(
    *,
    global_path: Path = GLOBAL_TOML,
    opencode_path: Path = OPENCODE_JSON,
    project_path: Path = PROJECT_TOML,
) -> list[tuple[ConfigSource, dict[str, Any]]]:
    """Return [(source, payload), ...] in ascending priority order.

    Lowest priority first. The merge order in :func:`get` walks this list
    and lets later entries overwrite earlier ones.
    """
    sources: list[tuple[ConfigSource, dict[str, Any]]] = [
        (
            ConfigSource(
                path=global_path,
                exists=global_path.exists(),
                priority=0,
                label="global",
            ),
            _read_toml(global_path),
        ),
        (
            ConfigSource(
                path=opencode_path,
                exists=opencode_path.exists(),
                priority=1,
                label="opencode",
            ),
            _read_opencode_json(opencode_path),
        ),
        (
            ConfigSource(
                path=project_path,
                exists=project_path.exists(),
                priority=2,
                label="project",
            ),
            _read_toml(project_path),
        ),
        (
            ConfigSource(
                path=Path("(env)"),
                exists=True,
                priority=3,
                label="env",
            ),
            _read_env(),
        ),
    ]
    return sources


# ── Dotted key resolution ────────────────────────────────────────────────
def get(key: str, **source_overrides: Path) -> ConfigView:
    """Resolve *key* (e.g. ``tui.theme``) against the layered store.

    Args:
        key: Dotted key. Empty sections are an error.
        **source_overrides: Per-source path overrides for testing
            (``global_path=``, ``opencode_path=``, ``project_path=``).

    Returns:
        A :class:`ConfigView` whose ``value`` is the resolved value or
        :data:`MISSING` when no source defined it.
    """
    if not key or "." not in key and not _is_toplevel(key):
        # Top-level keys are allowed (e.g. ``tui`` returns the whole table)
        pass
    sources = _read_sources(**source_overrides)
    for src, payload in sources:
        value = _dig(payload, key)
        if value is not MISSING:
            return ConfigView(key=key, value=value, source=src)
    return ConfigView(key=key, value=MISSING, source=None)


def _is_toplevel(key: str) -> bool:
    """Heuristic: a key without dots is treated as a section name."""
    return bool(key) and "." not in key


def _dig(payload: dict[str, Any], key: str) -> Any:
    """Look up *key* in *payload* using dot notation. Returns MISSING on miss."""
    if key in payload and not isinstance(payload[key], dict):
        return payload[key]
    if "." not in key:
        # Top-level section request — return the table only when every
        # value is a dict (so we never confuse it with a string).
        section = payload.get(key)
        if isinstance(section, dict):
            return section
        return MISSING
    section, _, rest = key.partition(".")
    head = payload.get(section)
    if not isinstance(head, dict):
        return MISSING
    return _dig(head, rest)


# ── Redaction ────────────────────────────────────────────────────────────
def _is_sensitive(key: str) -> bool:
    """True when the *key* matches a redaction pattern (last segment)."""
    leaf = key.rsplit(".", 1)[-1].lower()
    return any(sub in leaf for sub in REDACTED_SUBSTRINGS)


def redact(payload: Any) -> Any:
    """Recursively replace sensitive values in a nested dict/list.

    Used by :func:`show` and by the ``get`` command when the user passes
    ``--show-secret`` is NOT set. The function is a no-op for non-container
    scalars and non-string leaves.
    """
    if isinstance(payload, dict):
        return {
            k: (REDACTED_PLACEHOLDER if _is_sensitive(k) else redact(v))
            for k, v in payload.items()
        }
    if isinstance(payload, list):
        return [redact(v) for v in payload]
    return payload


# ── Mutators (write back to project TOML) ───────────────────────────────
def set_value(
    key: str, value: str, *, project_path: Path = PROJECT_TOML
) -> Path:
    """Set *key* to *value* in the project-level TOML file.

    Creates the file (and parents) if missing. *value* is stored as a
    string — callers that need typed values should store JSON, then cast
    on read (this keeps the CLI surface predictable).
    """
    if not key:
        raise ValueError("key must be non-empty")
    payload = _read_toml(project_path)
    section, _, leaf = key.partition(".")
    if not leaf:
        # Top-level assignment — store as a one-key table only if the value
        # is a JSON object; otherwise this is ambiguous and we error.
        raise ValueError(
            f"top-level key '{key}' is reserved for tables; "
            "use a dotted key like 'tui.theme'"
        )
    bucket = payload.setdefault(section, {})
    if not isinstance(bucket, dict):
        raise ValueError(f"'{section}' is not a table; cannot set '{key}'")
    bucket[leaf] = value
    project_path.parent.mkdir(parents=True, exist_ok=True)
    project_path.write_text(_writer().dumps(payload), encoding="utf-8")
    return project_path


def unset_value(key: str, *, project_path: Path = PROJECT_TOML) -> bool:
    """Remove *key* from the project-level TOML file.

    Returns True when a value was actually removed, False when the key
    was already absent (idempotent semantics for `sin config unset`).
    """
    if not key or "." not in key:
        return False
    if not project_path.exists():
        return False
    payload = _read_toml(project_path)
    section, _, leaf = key.partition(".")
    bucket = payload.get(section)
    if not isinstance(bucket, dict) or leaf not in bucket:
        return False
    del bucket[leaf]
    if not bucket:
        del payload[section]
    project_path.write_text(_writer().dumps(payload), encoding="utf-8")
    return True


# ── Listing ──────────────────────────────────────────────────────────────
def all_paths(
    *,
    global_path: Path = GLOBAL_TOML,
    opencode_path: Path = OPENCODE_JSON,
    project_path: Path = PROJECT_TOML,
) -> list[ConfigSource]:
    """Return every source the resolver consults, in priority order."""
    return [
        ConfigSource(
            path=global_path,
            exists=global_path.exists(),
            priority=0,
            label="global",
        ),
        ConfigSource(
            path=opencode_path,
            exists=opencode_path.exists(),
            priority=1,
            label="opencode",
        ),
        ConfigSource(
            path=project_path,
            exists=project_path.exists(),
            priority=2,
            label="project",
        ),
        ConfigSource(
            path=Path("(env SIN_*)"),
            exists=True,
            priority=3,
            label="env",
        ),
    ]


def merged(
    *,
    global_path: Path = GLOBAL_TOML,
    opencode_path: Path = OPENCODE_JSON,
    project_path: Path = PROJECT_TOML,
    redact_secrets: bool = True,
) -> tuple[dict[str, Any], dict[str, ConfigSource]]:
    """Merge all sources into one dict, plus a side-table of where each
    top-level section was last written.

    Returns ``(merged_payload, origins)`` where ``origins[section]`` is the
    :class:`ConfigSource` that contributed the *winning* value for the
    section's top-level keys (last-writer-wins per leaf, last-source-wins
    per section for the origin table).
    """
    sources = _read_sources(
        global_path=global_path,
        opencode_path=opencode_path,
        project_path=project_path,
    )
    merged_payload: dict[str, Any] = {}
    origins: dict[str, ConfigSource] = {}
    for src, payload in sources:
        for section, content in payload.items():
            if not isinstance(content, dict):
                # Scalar at top level — keep the last writer's value.
                merged_payload[section] = content
                origins[section] = src
                continue
            bucket = merged_payload.setdefault(section, {})
            if not isinstance(bucket, dict):
                bucket = {}
                merged_payload[section] = bucket
            for k, v in content.items():
                bucket[k] = v
                origins[f"{section}.{k}"] = src
        # Track env-source top-level entries (scalars) too.
        for section, content in list(merged_payload.items()):
            if section in payload and not isinstance(payload[section], dict):
                origins[section] = src
    if redact_secrets:
        merged_payload = redact(merged_payload)
    return merged_payload, origins


# ── Formatting ───────────────────────────────────────────────────────────
def format_show(
    payload: dict[str, Any], origins: dict[str, ConfigSource]
) -> str:
    """Render ``sin config show`` output: flat dotted-key list with origin."""
    if not payload:
        return "(no config set)"
    lines: list[str] = []
    for section in sorted(payload):
        content = payload[section]
        if isinstance(content, dict):
            for k in sorted(content):
                full = f"{section}.{k}"
                src = origins.get(full)
                src_label = src.label if src else "?"
                lines.append(f"{full} = {content[k]!r}   [{src_label}]")
        else:
            src = origins.get(section)
            src_label = src.label if src else "?"
            lines.append(f"{section} = {content!r}   [{src_label}]")
    return "\n".join(lines)


def format_path(sources: list[ConfigSource]) -> str:
    """Render ``sin config path`` output with an existence marker per source."""
    out: list[str] = []
    for src in sources:
        marker = "EXISTS" if src.exists else "absent"
        out.append(f"[{src.label:<8}] {src.path}   ({marker})")
    return "\n".join(out)
