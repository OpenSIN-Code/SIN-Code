"""Purpose: Tests for lib/sin_tools.py — wrapper for SIN-Code tool suite.

Docs: test_sin_tools.doc.md

Run: python3 -m pytest tests/test_sin_tools.py -v

The check_*() functions call scout/discover/map via subprocess. These
tests stub out call_sin_tool() with a fake that returns canned text,
so the tests are fast and hermetic. The full integration with the
real SIN-Code binaries is covered by tests/test_audit_end_to_end.py.
"""

import sys
from pathlib import Path
from unittest.mock import patch

# Make `lib/` importable without installing the skill as a package.
# Mirrors the layout: skills/code-skills/skill-code-ceo-audit/{lib,tests}/.
SKILL_DIR = Path(__file__).parent.parent
sys.path.insert(0, str(SKILL_DIR / "lib"))

import sin_tools  # noqa: E402

# ── Test fixtures (response shapes) ──────────────────────────────────


def _fake_sin_tool_response(text: str = "") -> dict:
    """Return a JSON-RPC response shape matching a real SIN-Code tool."""
    # Mirrors the actual JSON-RPC 2.0 envelope returned by sin-* binaries.
    return {
        "jsonrpc": "2.0",
        "id": 1,
        "result": {
            "content": [{"type": "text", "text": text}],
        },
    }


def _fake_with_match_lines(n: int = 3) -> str:
    """Build a scout-style output with `n` synthetic match lines."""
    # Each match line is recognised by count_matches() (contains "Match").
    return "\n".join(f"Match line {i}: foo" for i in range(n))


# ── existing API: call_sin_tool ───────────────────────────────────────


def test_call_sin_tool_returns_error_dict_on_missing_binary():
    """When the binary is not on PATH, returns {"error": ...} — never raises."""
    # shutil.which → None simulates the binary being absent.
    with patch("shutil.which", return_value=None):
        result = sin_tools.call_sin_tool("nonexistent-tool", {"path": "."})
    # Error path is a dict, NOT an exception — callers grep for "error".
    assert "error" in result
    assert "not installed" in result["error"]


def test_extract_text_returns_empty_on_error():
    """extract_text returns "" when the response is an error dict (no .result)."""
    # Error responses lack the .result.content list → must return "".
    assert sin_tools.extract_text({"error": "boom"}) == ""


def test_extract_text_reads_content_list():
    """extract_text pulls the text body out of a well-formed JSON-RPC response."""
    resp = _fake_sin_tool_response("hello world")
    assert sin_tools.extract_text(resp) == "hello world"


def test_count_matches_counts_match_lines():
    """count_matches is case-insensitive and counts each line containing "match"."""
    # count_matches is case-insensitive: "Match" OR "match" both count.
    text = "Match a\nno match\nMatch b\nalso no match\nmatch c"
    # 3 "Match" (case-sensitive) + 5 total (case-insensitive) = 5
    assert sin_tools.count_matches(text) == 5


def test_count_matches_zero_for_no_match_lines():
    """count_matches returns 0 when no line contains "match" (case-insensitive)."""
    # Negative path: no "match" substring anywhere → 0.
    assert sin_tools.count_matches("hello\nworld\nfoo") == 0


# ── per-axis checks: stub call_sin_tool ──────────────────────────────


def _patch_call_sin_tool(text: str):
    """Return a context-manager patcher for call_sin_tool to return `text`."""
    # Used by every check_*() test below to avoid real subprocess calls.
    return patch.object(
        sin_tools,
        "call_sin_tool",
        return_value=_fake_sin_tool_response(text),
    )


def test_check_security_returns_findings_when_scout_matches():
    """check_security emits findings when scout matches indicate hits."""
    # 5 match lines → at least one finding emitted.
    with _patch_call_sin_tool(_fake_with_match_lines(5)):
        result = sin_tools.check_security("/tmp/fake")
    assert "findings" in result
    assert len(result["findings"]) > 0
    # Each finding has the canonical shape
    for f in result["findings"]:
        # All six required keys must be present — schema lock-in.
        assert {"gate", "severity", "cwe", "title", "description", "fix"} <= set(f.keys())


def test_check_security_returns_empty_when_no_matches():
    """check_security emits zero findings when scout returns no matches."""
    # Empty scout output → no findings at all.
    with _patch_call_sin_tool(""):
        result = sin_tools.check_security("/tmp/fake")
    assert result["findings"] == []


def test_check_performance_respects_max_findings():
    """max_findings caps the number of findings even when many gates match."""
    # 20 match lines but cap=2 → exactly 2 findings max.
    with _patch_call_sin_tool(_fake_with_match_lines(20)):
        result = sin_tools.check_performance("/tmp/fake", max_findings=2)
    assert len(result["findings"]) <= 2


def test_check_quality_uses_discover_for_files():
    """check_quality uses discover first, then scout — verify both are called."""
    # Two patches: discover returns size hint, scout returns naming hits.
    with (
        patch.object(sin_tools, "discover", return_value="file.py: 600 lines") as m_disc,
        patch.object(sin_tools, "scout", return_value=""),
    ):
        result = sin_tools.check_quality("/tmp/fake")
    assert m_disc.called
    assert isinstance(result["findings"], list)


def test_check_testing_emits_info_when_no_test_framework():
    """check_testing surfaces the gate-4.1 INFO when no framework is detected."""
    with _patch_call_sin_tool(""):
        result = sin_tools.check_testing("/tmp/fake")
    # Should at least emit the "Test framework not detected" INFO finding
    assert any(f["gate"] == "4.1" for f in result["findings"])


def test_check_deps_flags_caret_versions():
    """check_deps emits gate-5.3 when caret/unpinned versions are present."""
    # Caret-version pattern in package.json/yarn.lock → MEDIUM finding.
    with _patch_call_sin_tool(_fake_with_match_lines(3)):
        result = sin_tools.check_deps("/tmp/fake")
    assert any(f["gate"] == "5.3" for f in result["findings"])


def test_check_docs_flags_missing_readme():
    """check_docs emits gate-6.1 when no README is discovered."""
    # discover for README returns empty → finding emitted
    with patch.object(sin_tools, "discover", return_value=""):
        result = sin_tools.check_docs("/tmp/fake")
    assert any(f["gate"] == "6.1" for f in result["findings"])


def test_check_architecture_handles_empty_map():
    """check_architecture returns a valid findings list even when map_arch is empty."""
    # Both map_arch and discover empty — function must NOT crash.
    with (
        patch.object(sin_tools, "map_arch", return_value=""),
        patch.object(sin_tools, "discover", return_value=""),
    ):
        result = sin_tools.check_architecture("/tmp/fake")
    assert "findings" in result
    assert isinstance(result["findings"], list)


def test_check_compliance_flags_missing_license():
    """check_compliance emits gate-8.1 and gate-8.2 when LICENSE/SECURITY are absent."""
    # No LICENSE or SECURITY.md → BOTH gates fire.
    with patch.object(sin_tools, "discover", return_value=""):
        result = sin_tools.check_compliance("/tmp/fake")
    assert any(f["gate"] == "8.1" for f in result["findings"])
    assert any(f["gate"] == "8.2" for f in result["findings"])


# ── dispatch ─────────────────────────────────────────────────────────


def test_check_axis_dispatches_by_name():
    """check_axis dispatches to the right axis function based on its name."""
    # Two valid axes → both return a {findings: ...} dict.
    with _patch_call_sin_tool(""):
        r1 = sin_tools.check_axis("security", "/tmp/fake")
        r2 = sin_tools.check_axis("performance", "/tmp/fake")
    assert "findings" in r1
    assert "findings" in r2


def test_check_axis_returns_error_for_unknown_axis():
    """check_axis returns an {"error": "unknown axis"} dict for an unknown name."""
    # Defensive default: unknown axis → soft error, NOT raise.
    result = sin_tools.check_axis("nonexistent-axis", "/tmp/fake")
    assert "error" in result
    assert "unknown axis" in result["error"]


def test_axis_checks_dict_has_all_eight_axes():
    """The 8 audit axes must all be present in AXIS_CHECKS."""
    # Pinned axis set — adding/removing one is a breaking change for axis_*.sh.
    expected = {
        "security",
        "performance",
        "quality",
        "testing",
        "deps",
        "docs",
        "architecture",
        "compliance",
    }
    assert set(sin_tools.AXIS_CHECKS.keys()) == expected
