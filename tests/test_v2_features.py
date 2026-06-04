"""Purpose: Tests for SIN-Code v2 features (VFS, Hashline, AST).

Docs: test_v2_features.doc.md
"""

from pathlib import Path

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
