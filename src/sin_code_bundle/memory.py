# SPDX-License-Identifier: MIT
"""SIN-Brain memory adapter (BR-1, Issue #14).

Thin, defensive bridge to the external ``sin_brain`` package. The bundle holds
**no** memory logic itself (that lives in SIN-Brain); this module only:

- detects whether ``sin_brain`` is importable and reports tier sizes for
  ``sin status`` (:func:`detect_env`), and
- exposes the five memory operations (:func:`recall`, :func:`remember`,
  :func:`forget`, :func:`pin`, :func:`link_evidence`) as thin pass-throughs that
  the MCP ``serve`` command registers as tools.

Every entry point degrades gracefully: if ``sin_brain`` is absent, detection
reports ``available=False`` and the operations raise :class:`MemoryUnavailable`,
which the caller turns into a clean tool-level error instead of crashing the
server.
"""

from __future__ import annotations

import importlib
import importlib.util
import json
from dataclasses import dataclass, field
from typing import Any

PACKAGE = "sin_brain"

# Canonical enums (kept in lock-step with the plan + AGENTS.md guidance).
RECALL_SCOPES = ("recall", "archival", "graph")
REMEMBER_KINDS = ("decision", "convention", "fix", "pitfall", "preference")
REMEMBER_SCOPES = ("repo", "user")
EVIDENCE_SOURCES = ("oracle", "poc", "ibd", "sckg", "adw")


class MemoryUnavailable(RuntimeError):
    """Raised when a memory operation is attempted without ``sin_brain``."""


@dataclass
class MemoryEnv:
    """Runtime availability snapshot for ``sin status``."""

    available: bool
    db_path: str | None = None
    tiers: dict[str, int] = field(default_factory=dict)
    detail: str = ""

    def to_dict(self) -> dict[str, Any]:
        return {
            "available": self.available,
            "db_path": self.db_path,
            "tiers": self.tiers,
            "detail": self.detail,
        }


def _tools_module():
    """Import ``sin_brain.mcp_tools`` or raise :class:`MemoryUnavailable`."""
    try:
        return importlib.import_module(f"{PACKAGE}.mcp_tools")
    except ImportError as exc:  # pragma: no cover - exercised via detect_env
        raise MemoryUnavailable(
            "sin-brain not installed. Install with: pip install sin-brain"
        ) from exc


def detect_env() -> MemoryEnv:
    """Report whether SIN-Brain is installed and, if so, its tier sizes."""
    if importlib.util.find_spec(PACKAGE) is None:
        return MemoryEnv(available=False, detail="sin_brain package not importable")
    try:
        mod = importlib.import_module(PACKAGE)
    except ImportError as exc:  # pragma: no cover
        return MemoryEnv(available=False, detail=f"import error: {exc}")

    db_path = None
    tiers: dict[str, int] = {}
    # SIN-Brain exposes an optional, cheap introspection hook. Treat any failure
    # as "available but stats unknown" rather than unavailable.
    stats = getattr(mod, "stats", None)
    if callable(stats):
        try:
            data = stats()
            db_path = data.get("db_path")
            tiers = data.get("tiers", {}) or {}
        except Exception as exc:  # noqa: BLE001 - never let stats break status
            return MemoryEnv(available=True, detail=f"stats unavailable: {exc}")
    return MemoryEnv(available=True, db_path=db_path, tiers=tiers, detail="ok")


# ── Operations — thin pass-throughs to sin_brain.mcp_tools (JSON-string results) ──
def recall(query: str, scope: str = "recall", k: int = 5) -> str:
    """Tiered memory search. Returns JSON: ids + snippets (not full docs)."""
    if scope not in RECALL_SCOPES:
        raise ValueError(f"scope must be one of {RECALL_SCOPES}")
    result = _tools_module().recall(query=query, scope=scope, k=k)
    return result if isinstance(result, str) else json.dumps(result)


def remember(content: str, kind: str, ttl_days: int | None = None, scope: str = "repo") -> str:
    """Self-editing memory write. Returns JSON with the new entry id."""
    if kind not in REMEMBER_KINDS:
        raise ValueError(f"kind must be one of {REMEMBER_KINDS}")
    if scope not in REMEMBER_SCOPES:
        raise ValueError(f"scope must be one of {REMEMBER_SCOPES}")
    result = _tools_module().remember(content=content, kind=kind, ttl_days=ttl_days, scope=scope)
    return result if isinstance(result, str) else json.dumps(result)


def forget(id: str) -> str:
    """Remove a memory entry. Returns JSON status."""
    result = _tools_module().forget(id=id)
    return result if isinstance(result, str) else json.dumps(result)


def pin(id: str) -> str:
    """Pin a memory entry so it is never evicted. Returns JSON status."""
    result = _tools_module().pin(id=id)
    return result if isinstance(result, str) else json.dumps(result)


def link_evidence(entity: str, verdict: str, source: str) -> str:
    """Attach a subsystem verdict to a code entity in the evidence graph."""
    if source not in EVIDENCE_SOURCES:
        raise ValueError(f"source must be one of {EVIDENCE_SOURCES}")
    result = _tools_module().link_evidence(entity=entity, verdict=verdict, source=source)
    return result if isinstance(result, str) else json.dumps(result)


def inject() -> str:
    """Return SIN-Brain's AGENTS.md inject block (SB-4), or '' if unavailable.

    Used by `sin agents-md` to embed the project's compiled memory context. The
    bundle owns no formatting here — SIN-Brain returns ready-to-embed Markdown.
    """
    if importlib.util.find_spec(PACKAGE) is None:
        return ""
    try:
        mod = importlib.import_module(PACKAGE)
    except ImportError:
        return ""
    fn = getattr(mod, "inject", None)
    if not callable(fn):
        return ""
    try:
        out = fn()
    except Exception:  # noqa: BLE001 - inject must never break callers
        return ""
    return out if isinstance(out, str) else ""


# ── MCP Registration (called by `sin serve`) ───────────────────────────────
# Kept here (not in cli.py) so the wiring is unit-testable with a fake MCP
# object and no `mcp` dependency.
TOOL_NAMES = ("recall", "remember", "forget", "pin", "link_evidence")


def register_tools(mcp: Any) -> list[str]:
    """Register the five memory tools on ``mcp`` if SIN-Brain is available.

    Returns the names registered (empty when sin-brain is absent) so callers and
    tests can assert on the wiring. Never raises on a missing package — graceful
    degradation is the contract.
    """
    if not detect_env().available:
        return []

    @mcp.tool()
    def recall_tool(query: str, scope: str = "recall", k: int = 5) -> str:
        """Search memory tiers (recall|archival|graph). Returns ids+snippets."""
        try:
            return recall(query, scope=scope, k=k)
        except (MemoryUnavailable, ValueError) as exc:
            return json.dumps({"error": str(exc)})

    @mcp.tool()
    def remember_tool(content: str, kind: str, ttl_days: int = 0, scope: str = "repo") -> str:
        """Persist a memory. kind: decision|convention|fix|pitfall|preference."""
        try:
            return remember(content, kind, ttl_days=ttl_days or None, scope=scope)
        except (MemoryUnavailable, ValueError) as exc:
            return json.dumps({"error": str(exc)})

    @mcp.tool()
    def forget_tool(id: str) -> str:
        """Delete a memory entry by id."""
        try:
            return forget(id)
        except MemoryUnavailable as exc:
            return json.dumps({"error": str(exc)})

    @mcp.tool()
    def pin_tool(id: str) -> str:
        """Pin a memory entry so it is never evicted."""
        try:
            return pin(id)
        except MemoryUnavailable as exc:
            return json.dumps({"error": str(exc)})

    @mcp.tool()
    def link_evidence_tool(entity: str, verdict: str, source: str) -> str:
        """Attach a subsystem verdict (oracle|poc|ibd|sckg|adw) to a code entity."""
        try:
            return link_evidence(entity, verdict, source)
        except (MemoryUnavailable, ValueError) as exc:
            return json.dumps({"error": str(exc)})

    return list(TOOL_NAMES)
