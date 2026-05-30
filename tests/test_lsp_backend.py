"""Tests for lsp_backend — primarily the tree-sitter fallback path,
since LSP servers won't be available in CI.
"""
from pathlib import Path


from sin_code_bundle.lsp_backend import (
    ImpactResult,
    Location,
    _is_public_api_path,
    _is_test_path,
    _score_risk,
    _treesitter_impact,
    compute_impact,
)


def test_score_risk_low():
    assert _score_risk(0, False, False) == "low"


def test_score_risk_medium_callers():
    assert _score_risk(5, False, False) == "medium"


def test_score_risk_high_api():
    assert _score_risk(1, False, True) == "high"


def test_score_risk_high_many_callers():
    assert _score_risk(11, False, False) == "high"


def test_is_test_path():
    assert _is_test_path("tests/test_foo.py")
    assert _is_test_path("foo_test.py")
    assert not _is_test_path("src/foo.py")


def test_is_public_api_path():
    assert _is_public_api_path("__init__.py")
    assert _is_public_api_path("api.py")
    assert _is_public_api_path("index.ts")
    assert not _is_public_api_path("utils.py")


def test_treesitter_finds_symbol(tmp_path: Path):
    src = tmp_path / "mymod.py"
    src.write_text(
        "def compute(x):\n    return x * 2\n\nresult = compute(5)\n",
        encoding="utf-8",
    )
    result = _treesitter_impact(tmp_path, "compute")
    assert result.symbol == "compute"
    assert result.defined_at is not None
    assert result.fan_in >= 1
    assert result.source == "treesitter"


def test_treesitter_unknown_symbol_returns_empty(tmp_path: Path):
    (tmp_path / "empty.py").write_text("x = 1\n", encoding="utf-8")
    result = _treesitter_impact(tmp_path, "nonexistent_symbol_xyz")
    assert result.fan_in == 0
    assert result.defined_at is None


def test_compute_impact_uses_cache(tmp_path: Path):
    src = tmp_path / "mod.py"
    src.write_text("def foo():\n    pass\n\nfoo()\n", encoding="utf-8")

    r1 = compute_impact(tmp_path, "foo")
    r2 = compute_impact(tmp_path, "foo")  # should hit cache
    assert r1.symbol == r2.symbol == "foo"
    assert r1.source == r2.source


def test_impact_result_to_dict():
    loc = Location(file="a.py", line=1, column=1, snippet="def foo():")
    result = ImpactResult(
        symbol="foo",
        defined_at=loc,
        callers=[loc],
        fan_in=1,
        touches_tests=False,
        touches_public_api=False,
        risk="low",
        source="treesitter",
        notes=["test"],
    )
    d = result.to_dict()
    assert d["symbol"] == "foo"
    assert d["fan_in"] == 1
    assert d["defined_at"]["file"] == "a.py"
    assert d["callers"][0]["line"] == 1
