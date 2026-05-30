"""Tests for the RTK bridge.

No real RTK invocation: discovery (shutil.which) and subprocess are stubbed so
the suite runs in CI without the rtk binary.
"""

from __future__ import annotations

import pytest

from sin_code_bundle import rtk


# --------------------------------------------------------------------------- #
# Environment discovery
# --------------------------------------------------------------------------- #
def test_env_unavailable_without_binary(monkeypatch):
    monkeypatch.setattr(rtk.shutil, "which", lambda _: None)
    env = rtk.detect_env()
    assert env.available is False
    with pytest.raises(rtk.RtkError):
        env.base_cmd()


def test_env_available(monkeypatch):
    monkeypatch.setattr(rtk.shutil, "which", lambda name: f"/usr/bin/{name}")
    env = rtk.detect_env()
    assert env.available is True
    assert env.base_cmd() == "/usr/bin/rtk"


# --------------------------------------------------------------------------- #
# init args matrix
# --------------------------------------------------------------------------- #
def test_init_args_per_agent():
    assert rtk.init_args("opencode") == ["init", "-g", "--opencode"]
    assert rtk.init_args("codex") == ["init", "-g", "--codex"]
    assert rtk.init_args("hermes") == ["init", "--agent", "hermes"]


def test_init_args_unknown_agent_raises():
    with pytest.raises(rtk.RtkError):
        rtk.init_args("nope")


# --------------------------------------------------------------------------- #
# setup_agents drives `rtk init`
# --------------------------------------------------------------------------- #
def test_setup_agents_runs_init_for_each(monkeypatch):
    monkeypatch.setattr(rtk.shutil, "which", lambda name: f"/usr/bin/{name}")
    calls = []

    class FakeProc:
        returncode = 0
        stdout = "ok"
        stderr = ""

    def fake_run(cmd, **kwargs):
        calls.append(cmd)
        return FakeProc()

    monkeypatch.setattr(rtk.subprocess, "run", fake_run)
    done = rtk.setup_agents(["opencode", "codex"])

    assert set(done) == {"opencode", "codex"}
    assert calls[0] == ["/usr/bin/rtk", "init", "-g", "--opencode"]
    assert calls[1] == ["/usr/bin/rtk", "init", "-g", "--codex"]


def test_setup_agents_raises_without_binary(monkeypatch):
    monkeypatch.setattr(rtk.shutil, "which", lambda _: None)
    with pytest.raises(rtk.RtkError):
        rtk.setup_agents(["opencode"])


def test_setup_agents_propagates_init_failure(monkeypatch):
    monkeypatch.setattr(rtk.shutil, "which", lambda name: f"/usr/bin/{name}")

    class FakeProc:
        returncode = 1
        stdout = ""
        stderr = "boom"

    monkeypatch.setattr(rtk.subprocess, "run", lambda cmd, **k: FakeProc())
    with pytest.raises(rtk.RtkError):
        rtk.setup_agents(["hermes"])


# --------------------------------------------------------------------------- #
# gain / doctor
# --------------------------------------------------------------------------- #
def test_gain_parses_json(monkeypatch):
    monkeypatch.setattr(rtk.shutil, "which", lambda name: f"/usr/bin/{name}")

    class FakeProc:
        returncode = 0
        stdout = '{"tokens_saved": 1234}'
        stderr = ""

    monkeypatch.setattr(rtk.subprocess, "run", lambda cmd, **k: FakeProc())
    assert rtk.gain() == {"tokens_saved": 1234}


def test_doctor_reports_availability(monkeypatch):
    monkeypatch.setattr(rtk.shutil, "which", lambda _: None)
    report = rtk.doctor()
    assert report["available"] is False
    assert "opencode" in report["agents"]
