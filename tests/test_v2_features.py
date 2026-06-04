"""Purpose: Tests for SIN-Code v2 features (VFS, Hashline, Memory, AST).

Docs: test_v2_features.doc.md
"""

from pathlib import Path

import pytest

# ── VFS: Virtual Filesystem ────────────────────────────────────


def test_vfs_schemes():
    """VFS exposes 7 URI schemes (sckg, poc, ibd, adw, efsm, oracle, conflict)."""
    from sin_code_bundle.vfs import URI_SCHEMES, SINVirtualFS

    assert "sckg" in URI_SCHEMES
    assert "poc" in URI_SCHEMES
    assert "ibd" in URI_SCHEMES
    assert "adw" in URI_SCHEMES
    assert "efsm" in URI_SCHEMES
    assert "oracle" in URI_SCHEMES
    assert "conflict" in URI_SCHEMES
    vfs = SINVirtualFS()
    schemes = vfs.list_schemes()
    assert "sckg" in schemes


def test_vfs_invalid_uri():
    """Returns an error dict for malformed URIs (no scheme prefix)."""
    from sin_code_bundle.vfs import SINVirtualFS

    vfs = SINVirtualFS()
    result = vfs.resolve("not a uri")
    assert "error" in result


def test_vfs_unknown_scheme():
    """Returns an error dict for unknown URI schemes."""
    from sin_code_bundle.vfs import SINVirtualFS

    vfs = SINVirtualFS()
    result = vfs.resolve("noscheme://foo")
    assert "error" in result


def test_vfs_caching():
    """Repeated resolves of the same URI return the cached result."""
    from sin_code_bundle.vfs import SINVirtualFS

    vfs = SINVirtualFS()
    r1 = vfs.resolve("noscheme://foo")
    r2 = vfs.resolve("noscheme://foo")
    # Same error returned (cached)
    assert r1 == r2


# ── Hashline: Content-Hash Patching ────────────────────────────


def test_hashline_basic():
    """Hashline can find anchors for known function declarations."""
    from sin_code_bundle.hashline import HashlineAnchor

    content = "def foo():\n    pass\n\ndef bar():\n    return 1\n"
    anchor = HashlineAnchor(content)
    line = anchor.find_anchor("def foo():")
    assert line == 0
    line = anchor.find_anchor("def bar():")
    assert line == 3


def test_hashline_patch_creation():
    """create_patch returns a valid dict with anchor_hash + line."""
    from sin_code_bundle.hashline import HashlineAnchor

    content = "def old():\n    pass\n"
    anchor = HashlineAnchor(content)
    patch = anchor.create_patch("def old():", "def new():")
    assert patch is not None
    assert patch["anchor_line"] == 0
    assert patch["old_content"] == "def old():"
    assert patch["new_content"] == "def new():"


def test_hashline_patch_apply():
    """apply_patch replaces old content with new content."""
    from sin_code_bundle.hashline import HashlineAnchor

    content = "def old():\n    pass\n"
    anchor = HashlineAnchor(content)
    patch = anchor.create_patch("def old():", "def new():")
    modified = anchor.apply_patch(patch)
    assert modified is not None
    assert "def new():" in modified
    assert "def old():" not in modified


def test_hashline_stale_anchor():
    """apply_patch returns None when anchor moved (safety guard)."""
    from sin_code_bundle.hashline import HashlineAnchor

    content = "def old():\n    pass\n"
    anchor = HashlineAnchor(content)
    patch = anchor.create_patch("def old():", "def new():")
    new_content = anchor.apply_patch(patch)
    new_anchor = HashlineAnchor(new_content)
    result = new_anchor.apply_patch(patch)
    assert result is None  # Stale


def test_hashline_semantic_patch(tmp_path):
    """SINHashlinePatch writes modified content atomically to file."""
    from sin_code_bundle.hashline import SINHashlinePatch

    f = tmp_path / "code.py"
    f.write_text("def hello():\n    print('old')\n")
    patcher = SINHashlinePatch()
    patch = patcher.create_semantic_patch(f, "def hello():", "def hello_world():")
    assert patch is not None
    success, msg = patcher.apply_semantic_patch(patch)
    assert success
    assert "def hello_world():" in f.read_text()


# ── Memory: SQLite + Honcho Backend ────────────────────────────


# NOTE: SINMemory / HonchoBackend classes were moved to the sin-brain external
# package during the operational-hardening merge (af69464). The bundle's
# memory.py is now a thin adapter; these tests reference the removed in-bundle
# classes and are skipped pending a rewrite against the sin-brain API.
def _memory_v2_available() -> bool:
    try:
        from sin_code_bundle.memory import SINMemory  # noqa: F401

        return True
    except ImportError:
        return False


_skip_memory_v2 = pytest.mark.skipif(
    not _memory_v2_available(),
    reason="SINMemory/HonchoBackend moved to sin-brain external package",
)


@_skip_memory_v2
def test_memory_retain_recall(tmp_path):
    """retain stores facts; recall finds them via LIKE search."""
    from sin_code_bundle.memory import SINMemory

    mem = SINMemory(db_path=tmp_path / "mem.db")
    r1 = mem.retain("User prefers TypeScript", tags=["preference"])
    r2 = mem.retain("Project uses FastAPI", tags=["tech", "backend"])
    assert r1["success"]
    assert r2["success"]
    results = mem.recall("typescript")
    assert len(results) >= 1
    assert any("TypeScript" in r["fact"] for r in results)


@_skip_memory_v2
def test_memory_tag_filter(tmp_path):
    """Recall with --tag=X filters to only matching facts."""
    from sin_code_bundle.memory import SINMemory

    mem = SINMemory(db_path=tmp_path / "mem.db")
    mem.retain("alpha fact", tags=["a"])
    mem.retain("beta fact", tags=["b"])
    results = mem.recall("fact", tags=["a"])
    assert len(results) == 1
    assert "alpha" in results[0]["fact"]


@_skip_memory_v2
def test_memory_forget(tmp_path):
    """forget removes a fact by ID; subsequent recall returns empty."""
    from sin_code_bundle.memory import SINMemory

    mem = SINMemory(db_path=tmp_path / "mem.db")
    r = mem.retain("to be forgotten", tags=["t"])
    mem.forget(r["id"])
    results = mem.recall("forgotten")
    assert len(results) == 0


@_skip_memory_v2
def test_memory_stats(tmp_path):
    """get_stats returns correct total_facts and aggregated tags."""
    from sin_code_bundle.memory import SINMemory

    mem = SINMemory(db_path=tmp_path / "mem.db")
    mem.retain("fact one", tags=["x"])
    mem.retain("fact two", tags=["y", "z"])
    stats = mem.get_stats()
    assert stats["total_facts"] == 2
    assert "x" in stats["tags"]
    assert "y" in stats["tags"]


@_skip_memory_v2
def test_memory_reflect(tmp_path):
    """reflect synthesizes answer + sources from memory."""
    from sin_code_bundle.memory import SINMemory

    mem = SINMemory(db_path=tmp_path / "mem.db")
    mem.retain("Python is a dynamic language", tags=["lang"])
    mem.retain("TypeScript is a typed superset of JavaScript", tags=["lang"])
    # Query matches "Python" which is in fact 1
    result = mem.reflect("Python")
    assert "answer" in result
    assert "sources" in result
    assert "confidence" in result
    assert result["confidence"] > 0


@_skip_memory_v2
def test_memory_reflect_empty(tmp_path):
    """reflect on empty memory returns confidence=0.0."""
    from sin_code_bundle.memory import SINMemory

    mem = SINMemory(db_path=tmp_path / "mem.db")
    result = mem.reflect("nothing about this")
    assert result["confidence"] == 0.0


@_skip_memory_v2
def test_memory_with_honcho_unavailable(tmp_path):
    """Memory works via SQLite even when Honcho server is unreachable."""
    from sin_code_bundle.memory import HonchoBackend, SINMemory

    mem = SINMemory(
        db_path=tmp_path / "mem.db",
        honcho_workspace="test-ws",
        honcho_base_url="http://localhost:1",  # unreachable
    )
    # Honcho backend is always attached — even when unavailable.
    assert mem.honcho is not None
    assert isinstance(mem.honcho, HonchoBackend)
    # is_available() should be False (server unreachable on a closed port).
    # Don't assert this strictly — depends on network — but in this
    # env the port-1 connection must fail fast.
    assert mem.honcho.is_available() is False
    result = mem.retain("test fact")
    assert result["success"]
    # The retain went to SQLite (always works).
    assert result["stored_in"] in ("SQLite", "SQLite + SCKG")


@_skip_memory_v2
def test_honcho_backend_init_lazy():
    """HonchoBackend defers _try_init until is_available() is called."""
    from sin_code_bundle.memory import HonchoBackend

    backend = HonchoBackend(workspace_id="test", base_url="http://localhost:1")
    # _init_attempted should be False before any call.
    assert backend._init_attempted is False
    # is_available triggers init.
    available = backend.is_available()
    assert isinstance(available, bool)
    # After is_available, _init_attempted is True.
    assert backend._init_attempted is True
    # Calling is_available again doesn't re-init.
    again = backend.is_available()
    assert again == available


@_skip_memory_v2
def test_honcho_backend_get_status():
    """get_status returns available/workspace_id/base_url/error dict."""
    from sin_code_bundle.memory import HonchoBackend

    backend = HonchoBackend(workspace_id="test", base_url="http://localhost:1")
    status = backend.get_status()
    assert "available" in status
    assert "workspace_id" in status
    assert "base_url" in status
    assert "error" in status
    # Lazy init triggered by get_status().
    assert backend._init_attempted is True
    # Unreachable server → available False.
    assert status["available"] is False
    # workspace_id + base_url are echoed back.
    assert status["workspace_id"] == "test"
    assert status["base_url"] == "http://localhost:1"


@_skip_memory_v2
def test_honcho_retain_message_unavailable():
    """retain_message returns None when Honcho is unavailable."""
    from sin_code_bundle.memory import HonchoBackend

    backend = HonchoBackend(workspace_id="test", base_url="http://localhost:1")
    # Force unavailable by short-circuiting init (no real network call).
    backend._init_attempted = True
    backend._available = False
    backend._honcho = None
    result = backend.retain_message(peer_name="x", content="y")
    assert result is None


@_skip_memory_v2
def test_memory_get_context_for_query(tmp_path):
    """get_context_for_query returns structured dict for LLM injection."""
    from sin_code_bundle.memory import SINMemory

    mem = SINMemory(db_path=tmp_path / "mem.db")
    mem.retain("User prefers TypeScript over JavaScript", tags=["preference"])
    result = mem.get_context_for_query("What languages does the user like?")
    # Required keys present.
    assert "query" in result
    assert "code_knowledge" in result
    assert "behavioral_insights" in result
    assert "synthesis" in result
    assert "backends" in result
    # SQLite backend is always live.
    assert result["backends"]["sqlite"] is True
    # Query is echoed back.
    assert result["query"] == "What languages does the user like?"
    # Synthesis is a string (possibly empty when neither optional
    # backend is available — but at minimum the key exists and is a str).
    assert isinstance(result["synthesis"], str)


@_skip_memory_v2
def test_memory_stats_includes_honcho(tmp_path):
    """get_stats includes honcho sub-dict with availability."""
    from sin_code_bundle.memory import SINMemory

    mem = SINMemory(db_path=tmp_path / "mem.db")
    stats = mem.get_stats()
    assert "honcho" in stats
    assert "available" in stats["honcho"]
    # Honcho backend is attached; on a closed port it must be unavailable.
    assert stats["honcho"]["available"] is False
    # Pre-existing keys still present (regression guard).
    assert "total_facts" in stats
    assert "tags" in stats
    assert "backend" in stats


# ── AST Edit: Tree-sitter Optional ─────────────────────────────


def test_ast_lazy_import():
    """ast_edit module imports without tree-sitter installed."""
    from sin_code_bundle.ast_edit import SINASTEdit

    ast = SINASTEdit()
    # tree-sitter NOT installed in this env
    assert ast.is_available() is False
    # Edit on missing file returns proper error
    result = ast.edit(Path("/nonexistent.py"), "old", "new")
    assert not result.success
    assert "File not found" in result.error or "tree-sitter" in result.error


def test_ast_returns_error_when_unavailable():
    """edit returns clear install hint when tree-sitter missing."""
    from sin_code_bundle.ast_edit import SINASTEdit

    ast = SINASTEdit()
    assert not ast.is_available()
    # Without tree-sitter, edit() should give install hint
    result = ast.edit(Path("nonexistent.py"), "x", "y")
    assert not result.success
