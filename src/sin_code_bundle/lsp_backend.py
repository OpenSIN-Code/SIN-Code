# SPDX-License-Identifier: MIT
"""LSP-backed symbol resolution for the SCKG.

This makes `impact()` structural and type-accurate instead of textual:
- "what calls this symbol?"  -> LSP references
- "where is it defined?"     -> LSP definition
- blast-radius scoring        -> ranked caller set + fan-in

Primary backend: multilspy (drives real language servers: pyright, gopls,
typescript-language-server, rust-analyzer, jdtls, …).
Fallback backend: tree-sitter symbol scan (cheap, language-agnostic, no server).

The module degrades gracefully: if no LSP is available it returns tree-sitter
results and flags `source="treesitter"`, so the agent still gets a useful signal
and the bundle keeps working (consistent with `sin status`).

Docs: lsp_backend.doc.md
"""

from __future__ import annotations

import asyncio
from dataclasses import dataclass, field
from pathlib import Path
from typing import Literal, Optional

Source = Literal["lsp", "treesitter", "none"]

_LANG_BY_EXT = {
    ".py": "python",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".js": "javascript",
    ".jsx": "javascript",
    ".go": "go",
    ".rs": "rust",
    ".java": "java",
    ".rb": "ruby",
    ".php": "php",
    ".cs": "csharp",
    ".c": "c",
    ".cpp": "cpp",
    ".h": "cpp",
}


@dataclass(frozen=True)
class Location:
    """A single source-code position, optionally with a short snippet."""

    file: str
    line: int
    column: int
    snippet: str = ""


# ── LSPBackend: Language Server Manager ────────────────────────────────
@dataclass
class ImpactResult:
    """Compact, deterministic blast-radius payload for the agent."""

    symbol: str
    defined_at: Optional[Location]
    callers: list[Location] = field(default_factory=list)
    fan_in: int = 0
    touches_tests: bool = False
    touches_public_api: bool = False
    risk: Literal["low", "medium", "high"] = "low"
    source: Source = "none"
    notes: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        """Serialize to a JSON-safe dict (caches the result under `cache.set`).

        Returns a plain dict with `Location` fields flattened to `{file, line,
        column, snippet}` so the GraphCache (JSONL-backed) can round-trip it
        without a custom encoder.
        """
        return {
            "symbol": self.symbol,
            "defined_at": _loc_to_dict(self.defined_at),
            "callers": [_loc_to_dict(c) for c in self.callers],
            "fan_in": self.fan_in,
            "touches_tests": self.touches_tests,
            "touches_public_api": self.touches_public_api,
            "risk": self.risk,
            "source": self.source,
            "notes": self.notes,
        }


def _loc_to_dict(loc: Optional[Location]) -> Optional[dict]:
    if loc is None:
        return None
    return {"file": loc.file, "line": loc.line, "column": loc.column, "snippet": loc.snippet}


def _lang_for(path: Path) -> Optional[str]:
    return _LANG_BY_EXT.get(path.suffix.lower())


def _score_risk(
    callers: int, touches_tests: bool, touches_api: bool
) -> Literal["low", "medium", "high"]:
    if touches_api or callers > 10:
        return "high"
    if touches_tests or callers > 3:
        return "medium"
    return "low"


def _is_test_path(p: str) -> bool:
    pl = p.lower()
    return "test" in Path(pl).name or "/tests/" in pl or pl.endswith("_test.py")


def _is_public_api_path(p: str) -> bool:
    name = Path(p).name.lower()
    return name in {"__init__.py", "api.py", "index.ts", "index.js", "mod.rs", "lib.rs"}


# ── Language Detection: File → Server Mapping ──────────────────────────
# --------------------------------------------------------------------------- #
# LSP backend (multilspy)
# --------------------------------------------------------------------------- #
async def _lsp_impact(
    root: Path, file: Path, symbol: str, line: int, column: int
) -> Optional[ImpactResult]:
    try:
        from multilspy import LanguageServer  # type: ignore
        from multilspy.multilspy_config import MultilspyConfig  # type: ignore
        from multilspy.multilspy_logger import MultilspyLogger  # type: ignore
    except ImportError:
        return None

    lang = _lang_for(file)
    if not lang:
        return None

    config = MultilspyConfig.from_dict({"code_language": lang})
    logger = MultilspyLogger()
    server = LanguageServer.create(config, logger, str(root))

    rel = str(file.relative_to(root)) if file.is_absolute() else str(file)
    async with server.start_server():
        definition = await server.request_definition(rel, line - 1, column - 1)
        references = await server.request_references(rel, line - 1, column - 1)

    def_loc: Optional[Location] = None
    if definition:
        d = definition[0]
        def_loc = Location(
            file=d.get("relativePath", d.get("uri", "")),
            line=d["range"]["start"]["line"] + 1,
            column=d["range"]["start"]["character"] + 1,
        )

    callers: list[Location] = []
    for ref in references or []:
        rp = ref.get("relativePath", ref.get("uri", ""))
        callers.append(
            Location(
                file=rp,
                line=ref["range"]["start"]["line"] + 1,
                column=ref["range"]["start"]["character"] + 1,
            )
        )

    touches_tests = any(_is_test_path(c.file) for c in callers)
    touches_api = any(_is_public_api_path(c.file) for c in callers)
    fan_in = len(callers)
    return ImpactResult(
        symbol=symbol,
        defined_at=def_loc,
        callers=callers[:25],
        fan_in=fan_in,
        touches_tests=touches_tests,
        touches_public_api=touches_api,
        risk=_score_risk(fan_in, touches_tests, touches_api),
        source="lsp",
        notes=[] if fan_in <= 25 else [f"{fan_in} callers total; showing first 25"],
    )


# --------------------------------------------------------------------------- #
# tree-sitter fallback (textual but symbol-aware)
# --------------------------------------------------------------------------- #
def _treesitter_impact(root: Path, symbol: str) -> ImpactResult:
    bare = symbol.split(".")[-1].split("::")[-1]
    callers: list[Location] = []
    defined_at: Optional[Location] = None

    for path in root.rglob("*"):
        if not path.is_file() or _lang_for(path) is None:
            continue
        if any(part in {".git", "node_modules", ".venv", "__pycache__"} for part in path.parts):
            continue
        try:
            text = path.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            continue
        for i, raw in enumerate(text.splitlines(), start=1):
            if bare not in raw:
                continue
            col = raw.find(bare) + 1
            loc = Location(
                file=str(path.relative_to(root)),
                line=i,
                column=col,
                snippet=raw.strip()[:120],
            )
            stripped = raw.lstrip()
            if defined_at is None and (
                stripped.startswith(("def ", "class ", "func ", "fn ", "function "))
                and bare in stripped.split("(")[0]
            ):
                defined_at = loc
            else:
                callers.append(loc)

    touches_tests = any(_is_test_path(c.file) for c in callers)
    touches_api = any(_is_public_api_path(c.file) for c in callers)
    fan_in = len(callers)
    return ImpactResult(
        symbol=symbol,
        defined_at=defined_at,
        callers=callers[:25],
        fan_in=fan_in,
        touches_tests=touches_tests,
        touches_public_api=touches_api,
        risk=_score_risk(fan_in, touches_tests, touches_api),
        source="treesitter",
        notes=["LSP unavailable — textual approximation. Install 'sin[lsp]' for accuracy."],
    )


# ── Graceful Shutdown: Cleanup Lifecycle ──────────────────────────────
# --------------------------------------------------------------------------- #
# Public entry point
# --------------------------------------------------------------------------- #
def compute_impact(
    root: str | Path,
    symbol: str,
    file: Optional[str | Path] = None,
    line: Optional[int] = None,
    column: Optional[int] = None,
) -> ImpactResult:
    """Resolve the blast radius of `symbol`.

    If (file, line, column) are given and an LSP is available, returns precise
    LSP references. Otherwise falls back to a tree-sitter/textual scan.

    Results are cached under .sin/cache/ and reused if the repo hasn't changed.
    """
    root_path = Path(root).resolve()

    # Cache layer
    from sin_code_bundle.cache import GraphCache

    cache = GraphCache(root_path)
    cache_key = f"impact:{symbol}:{file}:{line}:{column}"
    cached = cache.get(cache_key)
    if cached is not None:
        defined = cached.get("defined_at")
        return ImpactResult(
            symbol=cached["symbol"],
            defined_at=Location(**defined) if defined else None,
            callers=[Location(**c) for c in cached.get("callers", [])],
            fan_in=cached.get("fan_in", 0),
            touches_tests=cached.get("touches_tests", False),
            touches_public_api=cached.get("touches_public_api", False),
            risk=cached.get("risk", "low"),
            source=cached.get("source", "none"),
            notes=cached.get("notes", []),
        )

    if file and line and column:
        file_path = (
            (root_path / file) if not Path(file).is_absolute() else Path(file)  # type: ignore[arg-type]
        )
        try:
            result = asyncio.run(_lsp_impact(root_path, file_path, symbol, line, column))
            if result is not None:
                cache.set(cache_key, result.to_dict())
                return result
        except Exception as exc:  # noqa: BLE001
            ts = _treesitter_impact(root_path, symbol)
            ts.notes.append(f"LSP error, used fallback: {exc}")
            cache.set(cache_key, ts.to_dict())
            return ts

    result = _treesitter_impact(root_path, symbol)
    cache.set(cache_key, result.to_dict())
    return result
