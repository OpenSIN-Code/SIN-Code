# Purpose: Unified code archaeology — graph + cross-source context in 1 call.
# Docs: symbol_resolve.doc.md
"""Consolidates gitnexus_query + gitnexus_context + gitnexus_impact +
gitnexus_detect_changes. Also integrates sin-context-bridge for cross-source
context (memory, code knowledge graph).

Docs: symbol_resolve.doc.md
"""

from __future__ import annotations

import json
import shutil
import subprocess
from pathlib import Path
from typing import Any, Dict, List, Optional


# Default binary names we look up on PATH. Hard-coded fallback paths exist
# for the dev machine layout documented in AGENTS.md so the tool keeps
# working when PATH is restricted (e.g. inside the MCP stdio process).
_GITNEXUS_FALLBACK = "/Users/jeremy/Library/Python/3.14/bin/gitnexus"
_CONTEXT_BRIDGE_FALLBACK = "/Users/jeremy/Library/Python/3.14/bin/sin-context-bridge"


class SymbolResolver:
    """One-call code archaeology for any symbol.

    Fans out to gitnexus primitives + sin-context-bridge. Each source
    degrades independently — missing CLI or failing command leaves the
    result empty but does not raise.
    """

    def __init__(self, repo_root: Optional[Path] = None) -> None:
        self.repo_root = Path(repo_root) if repo_root else Path.cwd()

    def resolve(
        self,
        name: str,
        depth: int = 2,
        include: Optional[List[str]] = None,
    ) -> Dict[str, Any]:
        """Resolve a symbol via multiple graph queries.

        Args:
            name: function, class, or module name.
            depth: 1-3 levels of graph traversal.
            include: subset of {callers, callees, blast, recent, cross}.
                Defaults to all except ``cross``.

        Returns:
            Dict with per-source slices and a ``sources_queried`` list
            showing which CLIs responded successfully.
        """
        if include is None:
            include = ["callers", "callees", "blast", "recent"]

        # ── Resolve binaries ────────────────────────────────────────────
        # shutil.which is the canonical way to find a CLI on PATH; the
        # hard-coded fallbacks cover the dev-machine layout from AGENTS.md.
        gitnexus_bin = shutil.which("gitnexus") or _GITNEXUS_FALLBACK
        context_bridge_bin = shutil.which("sin-context-bridge") or _CONTEXT_BRIDGE_FALLBACK

        result: Dict[str, Any] = {
            "symbol": name,
            "depth": depth,
            "include": include,
            "callers": [],
            "callees": [],
            "blast_radius": {},
            "recent_changes": [],
            "cross_source": {},
            "sources_queried": [],
        }

        # ── 1. Callers + callees (gitnexus context) ─────────────────────
        # One CLI call feeds both slices — same JSON payload.
        if ("callers" in include or "callees" in include) and Path(gitnexus_bin).exists():
            try:
                proc = subprocess.run(
                    [gitnexus_bin, "context", name, "--json"],
                    capture_output=True,
                    text=True,
                    timeout=10,
                )
                if proc.returncode == 0 and proc.stdout.strip():
                    data = json.loads(proc.stdout)
                    if "callers" in include:
                        result["callers"] = data.get("callers", [])
                    if "callees" in include:
                        result["callees"] = data.get("callees", [])
                    result["sources_queried"].append("gitnexus:context")
            except (subprocess.TimeoutExpired, json.JSONDecodeError, Exception):
                # Graceful degradation — leave the slice empty.
                pass

        # ── 2. Blast radius (gitnexus impact) ───────────────────────────
        # Groups affected symbols by traversal depth for risk assessment.
        if "blast" in include and Path(gitnexus_bin).exists():
            try:
                proc = subprocess.run(
                    [gitnexus_bin, "impact", json.dumps({"target": name})],
                    capture_output=True,
                    text=True,
                    timeout=15,
                )
                if proc.returncode == 0 and proc.stdout.strip():
                    data = json.loads(proc.stdout)
                    affected = data.get("affected", [])
                    for item in affected:
                        d = item.get("depth", 1)
                        result["blast_radius"].setdefault(f"d{d}", []).append(item)
                    result["sources_queried"].append("gitnexus:impact")
            except (subprocess.TimeoutExpired, json.JSONDecodeError, Exception):
                pass

        # ── 3. Recent changes (gitnexus detect-changes) ─────────────────
        # Filters to changes mentioning the symbol — useful for "is this
        # currently being modified?" answers.
        if "recent" in include and Path(gitnexus_bin).exists():
            try:
                proc = subprocess.run(
                    [gitnexus_bin, "detect-changes", "--json"],
                    capture_output=True,
                    text=True,
                    timeout=10,
                )
                if proc.returncode == 0 and proc.stdout.strip():
                    data = json.loads(proc.stdout)
                    all_changes = data.get("changes", [])
                    # Filter cheaply by checking each change dict's JSON
                    # representation; cheaper than traversing every field.
                    result["recent_changes"] = [
                        c
                        for c in all_changes
                        if name.lower() in json.dumps(c).lower()
                    ]
                    result["sources_queried"].append("gitnexus:detect-changes")
            except (subprocess.TimeoutExpired, json.JSONDecodeError, Exception):
                pass

        # ── 4. Cross-source (sin-context-bridge) ────────────────────────
        # Bridges local SCKG + remote sin-brain + local memory into one view.
        if "cross" in include and Path(context_bridge_bin).exists():
            try:
                proc = subprocess.run(
                    [
                        context_bridge_bin,
                        "query",
                        name,
                        "--sources",
                        "sckg,sin_brain,local",
                    ],
                    capture_output=True,
                    text=True,
                    timeout=15,
                )
                if proc.returncode == 0 and proc.stdout.strip():
                    data = json.loads(proc.stdout)
                    result["cross_source"] = {
                        "query": name,
                        "chunks": data.get("chunks", []),
                        "sources_queried": data.get("sources_queried", []),
                    }
                    result["sources_queried"].append("sin-context-bridge")
            except (subprocess.TimeoutExpired, json.JSONDecodeError, Exception):
                pass

        return result
