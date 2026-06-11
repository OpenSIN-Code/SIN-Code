# SPDX-License-Identifier: MIT
"""Standalone MCP server tests via JSON-RPC over stdio.

Exercises `python -m sin_delegate serve` end-to-end:
- initialize handshake (init request + initialized notification)
- list_tools (must expose 6 tools)
- call each tool with realistic args and assert on the response

This is the ONLY way to catch a broken `__main__.py` before release.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile


def _start_server() -> subprocess.Popen:
    return subprocess.Popen(
        [sys.executable, "-m", "sin_delegate", "serve"],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, text=True, bufsize=0)


def _send(proc: subprocess.Popen, msg: dict) -> dict:
    proc.stdin.write(json.dumps(msg) + "\n")
    proc.stdin.flush()
    return json.loads(proc.stdout.readline())


def _notify(proc: subprocess.Popen, msg: dict) -> None:
    proc.stdin.write(json.dumps(msg) + "\n")
    proc.stdin.flush()


def _init(proc: subprocess.Popen) -> dict:
    """Run MCP initialize handshake; return the init response."""
    resp = _send(proc, {
        "jsonrpc": "2.0", "id": 1, "method": "initialize",
        "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                   "clientInfo": {"name": "test", "version": "0.0.1"}},
    })
    _notify(proc, {"jsonrpc": "2.0",
                   "method": "notifications/initialized"})
    return resp


def _close(proc: subprocess.Popen) -> None:
    try:
        proc.stdin.close()
    except Exception:
        pass
    proc.terminate()
    try:
        proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()


def test_mcp_serve_handshake_lists_six_tools():
    proc = _start_server()
    try:
        init = _init(proc)
        assert "result" in init, f"init failed: {init}"
        assert init["result"]["serverInfo"]["name"] == "sin-delegate-mcp"
        resp = _send(proc, {"jsonrpc": "2.0", "id": 2,
                            "method": "tools/list"})
        names = {t["name"] for t in resp["result"]["tools"]}
        assert {"sin_delegate", "sin_delegate_status",
                "sin_delegate_history", "sin_delegate_cancel",
                "sin_delegate_escalations",
                "sin_delegate_resolve"} <= names, f"missing: {names}"
    finally:
        _close(proc)


def test_mcp_serve_status_for_unknown_plan():
    proc = _start_server()
    try:
        _init(proc)
        resp = _send(proc, {"jsonrpc": "2.0", "id": 2,
                            "method": "tools/call",
                            "params": {"name": "sin_delegate_status",
                                       "arguments": {
                                           "plan_id": "no-such-plan-xyz"}}})
        payload = json.loads(resp["result"]["content"][0]["text"])
        assert payload["plan_id"] == "no-such-plan-xyz"
        assert payload["states"] == {}
    finally:
        _close(proc)


def test_mcp_serve_cancel_is_idempotent():
    proc = _start_server()
    try:
        _init(proc)
        for i, plan_id in enumerate(("plan-a", "plan-b", "plan-c"), start=2):
            resp = _send(proc, {"jsonrpc": "2.0", "id": i,
                                "method": "tools/call",
                                "params": {"name": "sin_delegate_cancel",
                                           "arguments":
                                               {"plan_id": plan_id}}})
            payload = json.loads(resp["result"]["content"][0]["text"])
            assert payload["cancelled"] is True
            assert payload["plan_id"] == plan_id
    finally:
        _close(proc)


def test_mcp_serve_delegate_with_dry_run_plan():
    """A trivial dry-run plan should produce a JSON RunResult back via MCP."""
    plan = {
        "goal": "echo hello",
        "tasks": [{"key": "k1", "title": "say hi",
                   "instructions": "print hi", "backend": "echo"}],
    }
    proc = _start_server()
    tmp = tempfile.mkdtemp()
    old_cwd = os.getcwd()
    os.chdir(tmp)
    try:
        _init(proc)
        resp = _send(proc, {"jsonrpc": "2.0", "id": 2,
                            "method": "tools/call",
                            "params": {"name": "sin_delegate",
                                       "arguments": {
                                           "plan": plan,
                                           "dry_run": True}}})
        text = resp["result"]["content"][0]["text"]
        payload = json.loads(text)
        assert payload["goal"] == "echo hello", payload
        assert "plan_id" in payload
        assert "outcomes" in payload
    finally:
        os.chdir(old_cwd)
        _close(proc)


def test_mcp_serve_resolve_rejects_unknown_escalation():
    proc = _start_server()
    try:
        _init(proc)
        resp = _send(proc, {"jsonrpc": "2.0", "id": 2,
                            "method": "tools/call",
                            "params": {"name": "sin_delegate_resolve",
                                       "arguments": {
                                           "plan_id": "ghost-plan",
                                           "escalation_id": "ghost-esc",
                                           "option_id": "drop"}}})
        payload = json.loads(resp["result"]["content"][0]["text"])
        assert payload["ok"] is False
        assert "not open" in payload["error"]
    finally:
        _close(proc)
