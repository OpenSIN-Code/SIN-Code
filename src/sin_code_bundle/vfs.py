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

from typing import Optional, Dict, Any
from pathlib import Path
import re
import subprocess


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
        self._cache: Dict[str, Any] = {}

    def resolve(self, uri: str) -> Dict[str, Any]:
        """Resolve a SIN URI to structured content."""
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
        try:
            from sin_code_poc import list_properties, property_metadata
            # Use the property registry for strategy listing
            props = property_metadata() if callable(property_metadata) else {}
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
        try:
            from sin_code_adw import smells
            # List available smell analyzers
            available = [m for m in dir(smells) if not m.startswith("_") and callable(getattr(smells, m))]
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
        try:
            from sin_code_efsm import services
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
        try:
            from sin_code_oracle import verifier
            available = [m for m in dir(verifier) if not m.startswith("_") and callable(getattr(verifier, m))]
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
            result = subprocess.run(
                ["git", "diff", "--name-only", "--diff-filter=U"],
                capture_output=True, text=True, cwd=self.repo_root, timeout=10,
            )
            conflicted = [f for f in result.stdout.splitlines() if f]
        except Exception as e:
            return {"error": f"git failed: {e}"}

        if path == "*" or path == "":
            return {"type": "conflict_bulk", "files": conflicted, "count": len(conflicted)}
        if path.isdigit():
            idx = int(path)
            if 0 <= idx < len(conflicted):
                return {"type": "conflict_single", "file": conflicted[idx]}
            return {"error": f"Conflict index {idx} out of range (have {len(conflicted)})"}
        return {"error": "Use conflict://* for all or conflict://<N> for specific"}


__all__ = ["SINVirtualFS", "URI_SCHEMES"]
