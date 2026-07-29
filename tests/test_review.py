from __future__ import annotations

import json
import sys
from types import SimpleNamespace

from typer.testing import CliRunner

from sin_code_bundle.cli import app
from sin_code_bundle.review import review_files

runner = CliRunner()


def test_markdown_review_uses_deterministic_text_fallback(tmp_path):
    before = tmp_path / "before.md"
    after = tmp_path / "after.md"
    before.write_text("# Before\n\nThis is `code`.\n", encoding="utf-8")
    after.write_text("# After\n\nThis isn't Python.\n", encoding="utf-8")

    result = review_files(before, after)

    assert result == {
        "intents": "Text changes detected: 2 lines added, 2 lines removed across 2 change hunks.",
        "risk": {
            "total_risk": 0.0,
            "factors": [],
            "hot_files": [],
            "breakdown": {},
        },
        "text": {
            "strategy": "deterministic-line-diff",
            "added_lines": 2,
            "removed_lines": 2,
            "changed_hunks": 2,
            "before_lines": 3,
            "after_lines": 3,
        },
    }


def test_mixed_python_markdown_review_uses_text_fallback(tmp_path, monkeypatch):
    before = tmp_path / "before.py"
    after = tmp_path / "after.md"
    before.write_text("def answer():\n    return 42\n", encoding="utf-8")
    after.write_text("# Answer\n\nReturns forty-two.\n", encoding="utf-8")

    class ExplodingASTDiff:
        def diff_files(self, *_args):
            raise AssertionError("mixed file types must not use the AST parser")

    monkeypatch.setitem(sys.modules, "sin_code_ibd", SimpleNamespace(ASTDiff=ExplodingASTDiff))

    result = review_files(before, after)

    assert result["text"]["strategy"] == "deterministic-line-diff"
    assert result["text"]["added_lines"] == 3
    assert result["text"]["removed_lines"] == 2


def test_python_review_preserves_ibd_behavior(tmp_path, monkeypatch):
    before = tmp_path / "before.py"
    after = tmp_path / "after.py"
    before.write_text("def answer():\n    return 41\n", encoding="utf-8")
    after.write_text("def answer():\n    return 42\n", encoding="utf-8")
    calls = []
    changes = [object()]

    class FakeASTDiff:
        def diff_files(self, file_a, file_b):
            calls.append((file_a, file_b))
            return changes

    class FakeIntentSummarizer:
        def summarize(self, received):
            assert received is changes
            return "AST summary"

    class FakeRiskScorer:
        def score(self, received):
            assert received is changes
            return {"total_risk": 0.7}

    monkeypatch.setitem(
        sys.modules,
        "sin_code_ibd",
        SimpleNamespace(
            ASTDiff=FakeASTDiff,
            IntentSummarizer=FakeIntentSummarizer,
            RiskScorer=FakeRiskScorer,
        ),
    )

    result = review_files(before, after)

    assert calls == [(str(before), str(after))]
    assert result == {"intents": "AST summary", "risk": {"total_risk": 0.7}}


def test_review_cli_handles_markdown_without_ibd(tmp_path):
    before = tmp_path / "before.md"
    after = tmp_path / "after.md"
    before.write_text("# Before\n", encoding="utf-8")
    after.write_text("# After\n", encoding="utf-8")

    result = runner.invoke(app, ["review", str(before), str(after)])

    assert result.exit_code == 0, result.output
    payload = json.loads(result.stdout)
    assert payload["text"] == {
        "strategy": "deterministic-line-diff",
        "added_lines": 1,
        "removed_lines": 1,
        "changed_hunks": 1,
        "before_lines": 1,
        "after_lines": 1,
    }
