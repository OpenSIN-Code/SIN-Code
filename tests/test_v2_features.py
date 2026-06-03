"""Purpose: Tests for SIN-Code v2 features (VFS, Hashline, Memory, AST).

Docs: test_v2_features.doc.md
"""
import json
import pytest
from pathlib import Path


def test_vfs_schemes():
    """VFS lists all schemes."""
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
    """Invalid URI returns error."""
    from sin_code_bundle.vfs import SINVirtualFS
    vfs = SINVirtualFS()
    result = vfs.resolve("not a uri")
    assert "error" in result


def test_vfs_unknown_scheme():
    """Unknown scheme returns error."""
    from sin_code_bundle.vfs import SINVirtualFS
    vfs = SINVirtualFS()
    result = vfs.resolve("noscheme://foo")
    assert "error" in result


def test_vfs_caching():
    """VFS caches results."""
    from sin_code_bundle.vfs import SINVirtualFS
    vfs = SINVirtualFS()
    r1 = vfs.resolve("noscheme://foo")
    r2 = vfs.resolve("noscheme://foo")
    # Same error returned (cached)
    assert r1 == r2


def test_hashline_basic():
    """Hashline can find anchors in content."""
    from sin_code_bundle.hashline import HashlineAnchor
    content = "def foo():\n    pass\n\ndef bar():\n    return 1\n"
    anchor = HashlineAnchor(content)
    line = anchor.find_anchor("def foo():")
    assert line == 0
    line = anchor.find_anchor("def bar():")
    assert line == 3


def test_hashline_patch_creation():
    """Hashline creates valid patches."""
    from sin_code_bundle.hashline import HashlineAnchor
    content = "def old():\n    pass\n"
    anchor = HashlineAnchor(content)
    patch = anchor.create_patch("def old():", "def new():")
    assert patch is not None
    assert patch["anchor_line"] == 0
    assert patch["old_content"] == "def old():"
    assert patch["new_content"] == "def new():"


def test_hashline_patch_apply():
    """Hashline applies patches atomically."""
    from sin_code_bundle.hashline import HashlineAnchor
    content = "def old():\n    pass\n"
    anchor = HashlineAnchor(content)
    patch = anchor.create_patch("def old():", "def new():")
    modified = anchor.apply_patch(patch)
    assert modified is not None
    assert "def new():" in modified
    assert "def old():" not in modified


def test_hashline_stale_anchor():
    """Stale anchors are rejected."""
    from sin_code_bundle.hashline import HashlineAnchor
    content = "def old():\n    pass\n"
    anchor = HashlineAnchor(content)
    patch = anchor.create_patch("def old():", "def new():")
    new_content = anchor.apply_patch(patch)
    new_anchor = HashlineAnchor(new_content)
    result = new_anchor.apply_patch(patch)
    assert result is None  # Stale


def test_hashline_semantic_patch(tmp_path):
    """SINHashlinePatch creates + applies patches to files."""
    from sin_code_bundle.hashline import SINHashlinePatch
    f = tmp_path / "code.py"
    f.write_text("def hello():\n    print('old')\n")
    patcher = SINHashlinePatch()
    patch = patcher.create_semantic_patch(f, "def hello():", "def hello_world():")
    assert patch is not None
    success, msg = patcher.apply_semantic_patch(patch)
    assert success
    assert "def hello_world():" in f.read_text()


def test_memory_retain_recall(tmp_path):
    """Memory can retain and recall facts."""
    from sin_code_bundle.memory import SINMemory
    mem = SINMemory(db_path=tmp_path / "mem.db")
    r1 = mem.retain("User prefers TypeScript", tags=["preference"])
    r2 = mem.retain("Project uses FastAPI", tags=["tech", "backend"])
    assert r1["success"]
    assert r2["success"]
    results = mem.recall("typescript")
    assert len(results) >= 1
    assert any("TypeScript" in r["fact"] for r in results)


def test_memory_tag_filter(tmp_path):
    """Memory recall filters by tag."""
    from sin_code_bundle.memory import SINMemory
    mem = SINMemory(db_path=tmp_path / "mem.db")
    mem.retain("alpha fact", tags=["a"])
    mem.retain("beta fact", tags=["b"])
    results = mem.recall("fact", tags=["a"])
    assert len(results) == 1
    assert "alpha" in results[0]["fact"]


def test_memory_forget(tmp_path):
    """Memory forget removes facts."""
    from sin_code_bundle.memory import SINMemory
    mem = SINMemory(db_path=tmp_path / "mem.db")
    r = mem.retain("to be forgotten", tags=["t"])
    mem.forget(r["id"])
    results = mem.recall("forgotten")
    assert len(results) == 0


def test_memory_stats(tmp_path):
    """Memory stats are correct."""
    from sin_code_bundle.memory import SINMemory
    mem = SINMemory(db_path=tmp_path / "mem.db")
    mem.retain("fact one", tags=["x"])
    mem.retain("fact two", tags=["y", "z"])
    stats = mem.get_stats()
    assert stats["total_facts"] == 2
    assert "x" in stats["tags"]
    assert "y" in stats["tags"]


def test_memory_reflect(tmp_path):
    """Memory reflect returns answer + sources."""
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


def test_memory_reflect_empty(tmp_path):
    """Memory reflect on empty returns confidence 0."""
    from sin_code_bundle.memory import SINMemory
    mem = SINMemory(db_path=tmp_path / "mem.db")
    result = mem.reflect("nothing about this")
    assert result["confidence"] == 0.0


def test_ast_lazy_import():
    """AST module is importable even without tree-sitter."""
    from sin_code_bundle.ast_edit import SINASTEdit, ASTEditResult
    ast = SINASTEdit()
    # tree-sitter NOT installed in this env
    assert ast.is_available() is False
    # Edit on missing file returns proper error
    result = ast.edit(Path("/nonexistent.py"), "old", "new")
    assert not result.success
    assert "File not found" in result.error or "tree-sitter" in result.error


def test_ast_returns_error_when_unavailable():
    """AST edit returns clear error when tree-sitter missing."""
    from sin_code_bundle.ast_edit import SINASTEdit
    ast = SINASTEdit()
    assert not ast.is_available()
    # Without tree-sitter, edit() should give install hint
    result = ast.edit(Path("nonexistent.py"), "x", "y")
    assert not result.success
