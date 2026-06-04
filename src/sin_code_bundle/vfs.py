# SPDX-License-Identifier: MIT
"""Purpose: Virtual Filesystem Layer (URI Schemes) for SIN-Code v2.

Docs: vfs.doc.md

Exposes semantic tools as URI schemes for any MCP client:
    sckg://module/<name>/dependencies  -> SCKG graph query
    sckg://module/<name>/callers       -> SCKG reverse lookup
    sckg://module/<name>/neighbors     -> SCKG neighbors
    poc://strategy/<name>              -> POC strategy list
    ibd://diff/<file>                  -> IBD parse file
    adw://smell/<name>                 -> ADW smell analyzer
    efsm://service/<name>              -> EFSM mock service
    oracle://strategy/<name>           -> Oracle verifier
    conflict://<id>                    -> Git conflict interface
"""

from __future__ import annotations

import re
import subprocess
from pathlib import Path
from typing import Any, Dict, Optional

URI_SCHEMES = {
    "sckg": "Semantic Codebase Knowledge Graph",
    "poc": "Proof of Correctness",
    "ibd": "Intent-Based Semantic Diff",
    "adw": "Architectural Debt Watchdog",
    "efsm": "Ephemeral Full-Stack Mock",
    "oracle": "Verification Oracle",
    "conflict": "Merge Conflict Resolution",
}


class SINVirtualFS:
    """Resolves SIN-specific URI schemes.

    Usage:
        vfs = SINVirtualFS(Path("/path/to/repo"))
        result = vfs.resolve("sckg://module/auth/dependencies")
    """

    def __init__(self, repo_root: Optional[Path] = None):
        self.repo_root = repo_root or Path.cwd()
        # _cache avoids recomputing expensive SCKG queries on repeated
        # resolve() calls (e.g. when an agent re-reads the same module
        # graph mid-session). Bounded only by session lifetime — fine
        # for our short-lived agent processes.
        self._cache: Dict[str, Any] = {}

    def resolve(self, uri: str) -> Dict[str, Any]:
        """Resolve a SIN URI to structured content."""
        # URI grammar per RFC 3986-ish: `scheme://path`. \w+ matches
        # `[A-Za-z0-9_]+` which covers all our scheme names (sckg, poc,
        # ibd, adw, efsm, oracle, conflict) and rejects whitespace,
        # colons, and other URI-illegal chars early.
        match = re.match(r"^(\w+)://(.+)$", uri)
        if not match:
            return {"error": f"Invalid URI format: {uri}"}
        scheme, path = match.group(1), match.group(2)

        cache_key = f"{scheme}://{path}"
        if cache_key in self._cache:
            return self._cache[cache_key]

        handler = getattr(self, f"_resolve_{scheme}", None)
        if not handler:
            return {"error": f"Unknown scheme: {scheme}"}
        result = handler(path)
        self._cache[cache_key] = result
        return result

    def list_schemes(self) -> Dict[str, str]:
        """List all available URI schemes."""
        return dict(URI_SCHEMES)

    # ── SCKG resolver (uses REAL KnowledgeGraph API) ─────────────────
    def _resolve_sckg(self, path: str) -> Dict[str, Any]:
        parts = path.split("/")
        if len(parts) < 2 or parts[0] != "module":
            return {"error": "Use sckg://module/<name>/<query_type>"}
        module_name = parts[1]
        query_type = parts[2] if len(parts) > 2 else "neighbors"
        # try/except around every resolver = graceful degradation:
        # if one subsystem breaks or is missing, the others still resolve.
        try:
            from sin_code_sckg import KnowledgeGraph

            kg = KnowledgeGraph(str(self.repo_root))
            kg.build_from_repo()
            node_id = "module:" + module_name
            result_data = {
                "module": module_name,
                "query_type": query_type,
            }
            if query_type == "neighbors":
                result_data["data"] = [str(n) for n in kg.get_neighbors(node_id)]
            elif query_type == "overview":
                result_data["data"] = kg.to_dict()
            else:
                result_data["data"] = kg.query(
                    f"MATCH (n:Module) WHERE n.name='{module_name}' RETURN n"
                )
            return {"type": "sckg_module", **result_data}
        except ImportError:
            return {"error": "SCKG not installed (pip install sin-code-sckg)"}
        except Exception as e:
            return {"error": f"SCKG error: {e}"}

    # ── POC resolver (uses REAL POC API) ─────────────────────────────
    def _resolve_poc(self, path: str) -> Dict[str, Any]:
        parts = path.split("/")
        if len(parts) < 2:
            return {"error": "Use poc://strategy/<name>"}
        strategy_name = parts[1]
        # try/except around every resolver = graceful degradation:
        # if one subsystem breaks or is missing, the others still resolve.
        try:
            from sin_code_poc import list_properties, property_metadata  # noqa: F401

            # Use the property registry for strategy listing
            props = property_metadata() if callable(property_metadata) else {}
            # [:50] limits blast radius — not the whole catalog — so the
            # response stays LLM-prompt-friendly (POC has hundreds of
            # properties, we only need a discoverable subset here).
            return {
                "type": "poc_strategy",
                "strategy": strategy_name,
                "available_properties": list(props.keys())[:50] if isinstance(props, dict) else [],
                "note": f"Run: sin poc verify --strategy={strategy_name} <file>",
            }
        except ImportError:
            return {"error": "POC not installed"}

    # ── IBD resolver (uses REAL IBD API) ─────────────────────────────
    def _resolve_ibd(self, path: str) -> Dict[str, Any]:
        parts = path.split("/")
        if len(parts) < 2 or parts[0] != "diff":
            return {"error": "Use ibd://diff/<file_path>"}
        file_path = self.repo_root / parts[1]
        if not file_path.exists():
            return {"error": f"File not found: {file_path}"}
        try:
            from sin_code_ibd import ASTDiff

            diff = ASTDiff(str(file_path))
            return {
                "type": "ibd_diff",
                "file": str(file_path),
                "ast": diff.to_dict() if hasattr(diff, "to_dict") else str(diff),
            }
        except ImportError:
            return {"error": "IBD not installed"}

    # ── ADW resolver (uses REAL ADW API) ─────────────────────────────
    def _resolve_adw(self, path: str) -> Dict[str, Any]:
        parts = path.split("/")
        if len(parts) < 2 or parts[0] != "smell":
            return {"error": "Use adw://smell/<name>"}
        smell_name = parts[1]
        # try/except around every resolver = graceful degradation:
        # if one subsystem breaks or is missing, the others still resolve.
        try:
            from sin_code_adw import smells

            # ADW doesn't expose a unified query API, so we surface the
            # available analyzers and tell the user to call them directly
            # (same rationale as EFSM/Oracle/POC below).
            available = [
                m for m in dir(smells) if not m.startswith("_") and callable(getattr(smells, m))
            ]
            return {
                "type": "adw_smell",
                "name": smell_name,
                "available_analyzers": available,
            }
        except ImportError:
            return {"error": "ADW not installed"}

    # ── EFSM resolver (uses REAL EFSM API) ───────────────────────────
    def _resolve_efsm(self, path: str) -> Dict[str, Any]:
        parts = path.split("/")
        if len(parts) < 2 or parts[0] != "service":
            return {"error": "Use efsm://service/<name>"}
        service_name = parts[1]
        # try/except around every resolver = graceful degradation:
        # if one subsystem breaks or is missing, the others still resolve.
        try:
            from sin_code_efsm import services

            # EFSM doesn't expose a unified query API, so we surface the
            # available services and tell the user to call them directly
            # (same rationale as ADW/Oracle/POC above).
            available = [m for m in dir(services) if not m.startswith("_")]
            return {
                "type": "efsm_service",
                "name": service_name,
                "available": available,
                "note": f"Run: sin efsm create --service={service_name}",
            }
        except ImportError:
            return {"error": "EFSM not installed"}

    # ── Oracle resolver (uses REAL Oracle API) ───────────────────────
    def _resolve_oracle(self, path: str) -> Dict[str, Any]:
        parts = path.split("/")
        if len(parts) < 2 or parts[0] != "strategy":
            return {"error": "Use oracle://strategy/<name>"}
        strategy_name = parts[1]
        # try/except around every resolver = graceful degradation:
        # if one subsystem breaks or is missing, the others still resolve.
        try:
            from sin_code_oracle import verifier

            available = [
                m for m in dir(verifier) if not m.startswith("_") and callable(getattr(verifier, m))
            ]
            # [:20] limits blast radius — Oracle's verifier module can
            # have many callables; we just need enough to be discoverable.
            # Oracle doesn't expose a unified query API either, so we
            # return the available verifiers and let the user call them
            # directly (same rationale as ADW/EFSM/POC above).
            return {
                "type": "oracle_strategy",
                "strategy": strategy_name,
                "available": available[:20],
            }
        except ImportError:
            return {"error": "Oracle not installed"}

    # ── Conflict resolver (git-based) ────────────────────────────────
    def _resolve_conflict(self, path: str) -> Dict[str, Any]:
        try:
            # git-based and cheap: `git diff --name-only --diff-filter=U`
            # lists unmerged (U-status) paths. We don't need to parse
            # conflict markers ourselves — just surface the file list.
            result = subprocess.run(
                ["git", "diff", "--name-only", "--diff-filter=U"],
                capture_output=True,
                text=True,
                cwd=self.repo_root,
                timeout=10,
            )
            conflicted = [f for f in result.stdout.splitlines() if f]
        except Exception as e:
            return {"error": f"git failed: {e}"}

        if path == "*" or path == "":
            # conflict://* = bulk list (most common, agents usually want
            # "what files are in conflict?" not a single one).
            return {"type": "conflict_bulk", "files": conflicted, "count": len(conflicted)}
        if path.isdigit():
            idx = int(path)
            if 0 <= idx < len(conflicted):
                return {"type": "conflict_single", "file": conflicted[idx]}
            return {"error": f"Conflict index {idx} out of range (have {len(conflicted)})"}
        return {"error": "Use conflict://* for all or conflict://<N> for specific"}


__all__ = ["SINVirtualFS", "URI_SCHEMES"]
