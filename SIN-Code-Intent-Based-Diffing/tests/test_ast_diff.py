import pytest

from sin_code_ibd.ast_diff import ASTDiff
from sin_code_ibd.intent_summarizer import IntentSummarizer
from sin_code_ibd.risk_scorer import RiskScorer

_ad = ASTDiff()
_skip = pytest.mark.skipif(
    not _ad.available, reason="tree-sitter python grammar not available"
)


@_skip
def test_signature_change():
    changes = _ad.diff_strings(b"def foo(x): pass", b"def foo(x, y): pass")
    assert any(c.change_type == "signature_changed" for c in changes)


@_skip
def test_added_symbol():
    changes = _ad.diff_strings(b"def foo(): pass", b"def foo(): pass\ndef bar(): pass")
    assert any(c.change_type == "added" for c in changes)


@_skip
def test_removed_symbol():
    changes = _ad.diff_strings(b"def foo(): pass\ndef bar(): pass", b"def foo(): pass")
    assert any(c.change_type == "removed" for c in changes)


@_skip
def test_intent_summarizer():
    changes = _ad.diff_strings(b"def foo(x): pass", b"def foo(x, y): pass")
    intents = IntentSummarizer().summarize(changes)
    assert any(i.category == "api_change" for i in intents)


@_skip
def test_risk_scoring():
    changes = _ad.diff_strings(b"def foo(x): pass", b"def foo(x, y): pass")
    score = RiskScorer().score(changes)
    assert score["risk"] in ("low", "medium", "high")
    assert score["change_count"] == len(changes)


def test_summarizer_empty():
    assert IntentSummarizer().summarize([]) == []


def test_risk_empty():
    assert RiskScorer().score([])["risk"] == "low"
