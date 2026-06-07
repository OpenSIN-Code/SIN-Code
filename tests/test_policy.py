# SPDX-License-Identifier: MIT
from pathlib import Path

import pytest

from sin_code_bundle.policy import (
    AuditLog,
    Policy,
    PolicyError,
    ensure_within_root,
    guarded,
)


def test_default_read_allows():
    p = Policy()
    assert p.decide("impact") == "allow"
    assert p.decide("semantic_diff") == "allow"


def test_default_exec_asks():
    p = Policy()
    assert p.decide("verify_tests") == "ask"


def test_default_network_asks():
    p = Policy()
    assert p.decide("mock_env") == "ask"


def test_unknown_tool_treated_as_exec():
    p = Policy()
    assert p.decide("some_unknown_tool") == "ask"


def test_guarded_allows_read(tmp_path: Path):
    out = guarded("impact", {"symbol": "x"}, lambda: {"ok": True}, root=tmp_path)
    assert out == {"ok": True}


def test_guarded_denies_without_approval(tmp_path: Path):
    with pytest.raises(PolicyError, match="requires approval"):
        guarded("verify_tests", {}, lambda: {"ok": True}, root=tmp_path)


def test_guarded_allows_exec_with_auto_approve(tmp_path: Path, monkeypatch):
    monkeypatch.setenv("SIN_AUTO_APPROVE", "1")
    p = Policy()
    assert p.auto_approve is True
    # guarded should succeed when auto_approve is on
    out = guarded(
        "verify_tests",
        {},
        lambda: {"ok": True},
        root=tmp_path,
        approver=None,
    )
    assert out == {"ok": True}


def test_audit_chain_intact(tmp_path: Path):
    log = AuditLog(tmp_path)
    log.record("impact", {"symbol": "x"}, "allow", "ok")
    log.record("verify_tests", {}, "ask", "ok")
    assert log.verify_chain() is True


def test_audit_chain_empty(tmp_path: Path):
    log = AuditLog(tmp_path)
    assert log.verify_chain() is True


def test_audit_chain_detects_tampering(tmp_path: Path):
    log = AuditLog(tmp_path)
    log.record("impact", {"symbol": "x"}, "allow", "ok")
    text = log.path.read_text(encoding="utf-8").replace('"ok"', '"HACKED"')
    log.path.write_text(text, encoding="utf-8")
    assert log.verify_chain() is False


def test_path_sandbox_inside(tmp_path: Path):
    inside = ensure_within_root("sub/file.py", root=tmp_path)
    assert str(inside).startswith(str(tmp_path.resolve()))


def test_path_sandbox_outside_raises(tmp_path: Path):
    with pytest.raises(PolicyError, match="outside project root"):
        ensure_within_root("/etc/passwd", root=tmp_path)
