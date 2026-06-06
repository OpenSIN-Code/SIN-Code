#!/usr/bin/env python3
"""Fuzz test runner for any Go MCP tool binary.

Sends malformed JSON-RPC 2.0 requests via stdin/subprocess,
detects crashes, hangs, and protocol violations.

Usage:
    python3 fuzz_go_mcp.py /Users/jeremy/.local/bin/discover --label discover
"""

from __future__ import annotations

import json
import subprocess
import sys
import time
import os
import argparse
import signal
from dataclasses import dataclass, field
from typing import Any


@dataclass
class AttackResult:
    name: str
    payload: Any
    crashed: bool = False
    hung: bool = False
    exit_code: int | None = None
    error_msg: str = ""
    response_json: Any = None
    had_response: bool = False
    protocol_violation: str = ""


@dataclass
class FuzzReport:
    tool_name: str
    binary_path: str
    results: list[AttackResult] = field(default_factory=list)

    @property
    def crashes(self) -> list[AttackResult]:
        return [r for r in self.results if r.crashed]

    @property
    def hangs(self) -> list[AttackResult]:
        return [r for r in self.results if r.hung]

    @property
    def violations(self) -> list[AttackResult]:
        return [r for r in self.results if r.protocol_violation]

    @property
    def total(self) -> int:
        return len(self.results)


# ── Payload generators ──────────────────────────────────

def make_empty_inputs():
    return [
        ("empty_string", b""),
        ("whitespace_only", b"   "),
        ("just_newline", b"\n"),
        ("null_byte", b"\x00"),
        ("many_newlines", b"\n\n\n\n\n"),
    ]


def make_non_json():
    return [
        ("plain_text", b"not json"),
        ("html", b"<html></html>"),
        ("binary_data", bytes(range(256))),
        ("sql_injection", b"'; DROP TABLE users;--"),
        ("random_bytes", os.urandom(1024)),
    ]


def make_invalid_json_rpc():
    return [
        ("empty_object", json.dumps({}).encode()),
        ("wrong_jsonrpc_v1", json.dumps({"jsonrpc": "1.0"}).encode()),
        ("empty_jsonrpc", json.dumps({"jsonrpc": ""}).encode()),
        ("null_jsonrpc", json.dumps({"jsonrpc": None}).encode()),
        ("jsonrpc_as_number", json.dumps({"jsonrpc": 2.0}).encode()),
        ("missing_jsonrpc", json.dumps({"method": "tools/list", "id": 1}).encode()),
    ]


def make_missing_method():
    return [
        ("missing_method", json.dumps({"jsonrpc": "2.0", "id": 1, "params": {}}).encode()),
        ("null_method", json.dumps({"jsonrpc": "2.0", "method": None, "id": 1}).encode()),
    ]


def make_weird_method_names():
    return [
        ("unicode_emoji", json.dumps({"jsonrpc": "2.0", "method": "\U0001f4a9", "id": 1}).encode()),
        ("unicode_chinese", json.dumps({"jsonrpc": "2.0", "method": "\u4f60\u597d\u4e16\u754c", "id": 2}).encode()),
        ("special_chars", json.dumps({"jsonrpc": "2.0", "method": "tool/!@#$%^&*()", "id": 3}).encode()),
        ("sql_injection_method", json.dumps({"jsonrpc": "2.0", "method": "\"; DROP TABLE;--", "id": 4}).encode()),
        ("empty_method", json.dumps({"jsonrpc": "2.0", "method": "", "id": 5}).encode()),
    ]


def make_long_method_name():
    name_10kb = "a" * (10 * 1024)    # 10 KB
    name_100kb = "a" * (100 * 1024)  # 100 KB
    return [
        ("long_method_10kb", json.dumps({"jsonrpc": "2.0", "method": name_10kb, "id": 1}).encode()),
        ("long_method_100kb", json.dumps({"jsonrpc": "2.0", "method": name_100kb, "id": 1}).encode()),
    ]


def make_missing_params():
    return [
        ("missing_params", json.dumps({"jsonrpc": "2.0", "method": "tools/list", "id": 1}).encode()),
    ]


def make_weird_params():
    return [
        ("params_as_array", json.dumps({"jsonrpc": "2.0", "method": "tools/call", "id": 1, "params": [1, 2, 3]}).encode()),
        ("params_as_string", json.dumps({"jsonrpc": "2.0", "method": "tools/call", "id": 1, "params": "hello"}).encode()),
        ("params_as_number", json.dumps({"jsonrpc": "2.0", "method": "tools/call", "id": 1, "params": 42}).encode()),
        ("params_as_null", json.dumps({"jsonrpc": "2.0", "method": "tools/call", "id": 1, "params": None}).encode()),
    ]


def make_nested_objects(depth=1000):
    obj = {}
    current = obj
    for i in range(depth):
        current["nested"] = {}
        current = current["nested"]
    current["value"] = "bottom"
    return [
        ("nested_1000_levels", json.dumps({"jsonrpc": "2.0", "method": "tools/call", "id": 1, "params": obj}).encode()),
    ]


def make_weird_ids():
    return [
        ("negative_id", json.dumps({"jsonrpc": "2.0", "method": "tools/list", "id": -1}).encode()),
        ("float_id", json.dumps({"jsonrpc": "2.0", "method": "tools/list", "id": 3.14}).encode()),
        ("string_id", json.dumps({"jsonrpc": "2.0", "method": "tools/list", "id": "abc"}).encode()),
        ("null_id", json.dumps({"jsonrpc": "2.0", "method": "tools/list", "id": None}).encode()),
        ("very_large_id", json.dumps({"jsonrpc": "2.0", "method": "tools/list", "id": 2**63}).encode()),
    ]


def make_batch_requests():
    batch_good = [{"jsonrpc": "2.0", "method": "tools/list", "id": 1}]
    batch_mixed = [
        {"jsonrpc": "2.0", "method": "tools/list", "id": 1},
        "not a jsonrpc object",
        {"jsonrpc": "2.0", "method": None, "id": 2},
        {},
        {"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "recall", "arguments": {}}},
    ]
    batch_empty = []
    return [
        ("batch_all_good", json.dumps(batch_good).encode()),
        ("batch_mixed", json.dumps(batch_mixed).encode()),
        ("batch_empty", json.dumps(batch_empty).encode()),
    ]


def make_notifications():
    return [
        ("notification_no_id", json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}).encode()),
        ("notification_params", json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized", "params": {"key": "val"}}).encode()),
    ]


def make_oversized():
    """10 MB request body filled with padding."""
    padding = "x" * (10 * 1024 * 1024 - 100)
    payload = {"jsonrpc": "2.0", "method": "tools/list", "id": 1, "params": {"pad": padding}}
    return [
        ("oversized_10mb", json.dumps(payload).encode()),
    ]


def make_invalid_json_syntax():
    return [
        ("unclosed_brace", b'{"jsonrpc":"2.0","method":"tools/list"'),
        ("unclosed_bracket", b'[{"jsonrpc":"2.0","method":"tools/list","id":1}'),
        ("trailing_comma", b'{"jsonrpc":"2.0","method":"tools/list","id":1,}'),
        ("single_quote", b"{'jsonrpc':'2.0','method':'tools/list','id':1}"),
        ("unquoted_key", b'{jsonrpc:"2.0",method:"tools/list",id:1}'),
        ("starts_with_garbage", b'garbage{"jsonrpc":"2.0"}'),
    ]


# ── Response validation ────────────────────────────────

def validate_jsonrpc_response(response_json: Any) -> str:
    """Check JSON-RPC 2.0 compliance. Returns violation description or ""."""
    if not isinstance(response_json, dict):
        if isinstance(response_json, list):
            # Batch response - check each item
            for item in response_json:
                violation = validate_jsonrpc_response(item)
                if violation:
                    return f"batch_item: {violation}"
            return ""
        return f"response_not_object: {type(response_json).__name__}"

    # Must have jsonrpc: "2.0"
    if "jsonrpc" not in response_json:
        return "missing_jsonrpc_field"
    if response_json["jsonrpc"] != "2.0":
        return f"bad_jsonrpc_value: {response_json['jsonrpc']}"

    # id field handling - note notifications don't get responses,
    # but if we get a response it must have id or error
    has_id = "id" in response_json
    has_result = "result" in response_json
    has_error = "error" in response_json

    if has_error:
        err = response_json["error"]
        if not isinstance(err, dict):
            return "error_not_object"
        if "code" not in err or not isinstance(err["code"], int):
            return "error_missing_integer_code"
        if "message" not in err or not isinstance(err["message"], str):
            return "error_missing_string_message"

    # Must have either result or error, not both, not neither
    if has_result and has_error:
        return "both_result_and_error"
    if not has_result and not has_error:
        return "neither_result_nor_error"

    return ""


# ── Core fuzzing logic ─────────────────────────────────

def run_single_attack(
    binary: str, name: str, payload: bytes, timeout: float = 5.0
) -> AttackResult:
    """Send one attack payload via subprocess to a Go MCP binary."""
    result = AttackResult(name=name, payload=repr(payload[:100]))

    try:
        proc = subprocess.Popen(
            [binary, "--mcp"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )

        # Send payload + newline separator (MCP stdio uses \n-delimited JSON)
        try:
            proc.stdin.write(payload + b"\n")
            proc.stdin.flush()
        except (BrokenPipeError, OSError) as e:
            result.crashed = True
            result.error_msg = f"broken_pipe_on_write: {e}"
            proc.wait()
            result.exit_code = proc.returncode
            return result

        # Read response with timeout
        try:
            stdout_data, stderr_data = proc.communicate(timeout=timeout)
            result.exit_code = proc.returncode
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
            result.hung = True
            result.error_msg = f"timeout after {timeout}s (process killed)"
            result.exit_code = -signal.SIGKILL
            return result

        # Check if process crashed (non-zero exit)
        if proc.returncode != 0:
            result.crashed = True
            result.error_msg = f"exit_code={proc.returncode}, stderr={stderr_data[:200]!r}"
            # Still try to parse any stdout we got
            if stdout_data.strip():
                result.had_response = True
                try:
                    result.response_json = json.loads(stdout_data)
                except json.JSONDecodeError:
                    pass
            return result

        # Parse response
        if stdout_data.strip():
            result.had_response = True
            try:
                result.response_json = json.loads(stdout_data)
                violation = validate_jsonrpc_response(result.response_json)
                if violation:
                    result.protocol_violation = violation
            except json.JSONDecodeError as e:
                result.protocol_violation = f"non_json_response: {stdout_data[:200]!r}"

    except FileNotFoundError:
        result.crashed = True
        result.error_msg = f"binary_not_found: {binary}"
    except Exception as e:
        result.crashed = True
        result.error_msg = f"exception: {type(e).__name__}: {e}"

    return result


def fuzz_go_tool(binary: str, tool_label: str, timeout: float = 5.0) -> FuzzReport:
    """Run all attack vectors against a single Go MCP binary."""
    report = FuzzReport(tool_name=tool_label, binary_path=binary)

    # Verify binary exists
    if not os.path.exists(binary):
        print(f"  ❌ Binary not found: {binary}")
        report.results.append(AttackResult(name="binary_check", payload=None, crashed=True,
                                            error_msg="binary_not_found"))
        return report

    # Quick startup test: valid tools/list
    print(f"  → Startup check ({tool_label})...")
    startup = run_single_attack(
        binary, "startup_check",
        json.dumps({"jsonrpc": "2.0", "method": "tools/list", "id": 1}).encode(),
        timeout=timeout,
    )
    report.results.append(startup)
    if startup.crashed or startup.hung:
        print(f"  ⚠️  Startup failed: crash={startup.crashed}, hang={startup.hung}")
    else:
        print(f"  ✅ Startup OK")

    all_attacks = []

    # ── Category 1: Empty/invalid raw input ────────────
    print(f"  → Testing empty/malformed raw input...")
    for name, payload in make_empty_inputs():
        all_attacks.append((name, payload))
    for name, payload in make_non_json():
        all_attacks.append((name, payload))
    for name, payload in make_invalid_json_syntax():
        all_attacks.append((name, payload))

    # ── Category 2: Invalid JSON-RPC structure ─────────
    print(f"  → Testing invalid JSON-RPC...")
    for name, payload in make_invalid_json_rpc():
        all_attacks.append((name, payload))
    for name, payload in make_missing_method():
        all_attacks.append((name, payload))
    for name, payload in make_weird_method_names():
        all_attacks.append((name, payload))
    for name, payload in make_weird_ids():
        all_attacks.append((name, payload))

    # ── Category 3: Weird params ───────────────────────
    print(f"  → Testing weird params...")
    for name, payload in make_missing_params():
        all_attacks.append((name, payload))
    for name, payload in make_weird_params():
        all_attacks.append((name, payload))

    # ── Category 4: Long inputs ────────────────────────
    print(f"  → Testing long method names...")
    for name, payload in make_long_method_name():
        all_attacks.append((name, payload))

    # ── Category 5: Deep nesting ───────────────────────
    print(f"  → Testing deep nesting...")
    for name, payload in make_nested_objects(1000):
        all_attacks.append((name, payload))

    # ── Category 6: Batches ────────────────────────────
    print(f"  → Testing batch requests...")
    for name, payload in make_batch_requests():
        all_attacks.append((name, payload))

    # ── Category 7: Notifications ──────────────────────
    print(f"  → Testing notifications...")
    for name, payload in make_notifications():
        all_attacks.append((name, payload))

    # ── Category 8: Oversized ──────────────────────────
    print(f"  → Testing oversized payload (10MB)...")
    for name, payload in make_oversized():
        all_attacks.append((name, payload))

    print(f"  → Running {len(all_attacks)} attacks...")
    for i, (name, payload) in enumerate(all_attacks):
        result = run_single_attack(binary, name, payload, timeout=timeout)
        report.results.append(result)

        # Progress indicator
        if result.crashed:
            print(f"    [{i+1}/{len(all_attacks)}] 💥 {name}: CRASH (exit={result.exit_code})")
        elif result.hung:
            print(f"    [{i+1}/{len(all_attacks)}] ⏳ {name}: HANG (>5s)")
        elif result.protocol_violation:
            print(f"    [{i+1}/{len(all_attacks)}] ⚠️  {name}: protocol violation ({result.protocol_violation})")
        else:
            print(f"    [{i+1}/{len(all_attacks)}] ✅ {name}: OK")

    return report


def print_report(report: FuzzReport):
    """Print a summary report."""
    print()
    print("=" * 72)
    print(f"  FUZZ REPORT: {report.tool_name}")
    print(f"  Binary: {report.binary_path}")
    print("=" * 72)
    print(f"  Total attacks:  {report.total}")
    print(f"  💥 Crashes:      {len(report.crashes)}")
    print(f"  ⏳ Hangs:        {len(report.hangs)}")
    print(f"  ⚠️  Violations:   {len(report.violations)}")

    if report.crashes:
        print()
        print("  ── CRASHES ──")
        for r in report.crashes:
            print(f"    💥 {r.name}: {r.error_msg}")

    if report.hangs:
        print()
        print("  ── HANGS ──")
        for r in report.hangs:
            print(f"    ⏳ {r.name}: {r.error_msg}")

    if report.violations:
        print()
        print("  ── PROTOCOL VIOLATIONS ──")
        for r in report.violations:
            print(f"    ⚠️  {r.name}: {r.protocol_violation}")
            if r.response_json:
                print(f"       Response: {json.dumps(r.response_json)[:200]}")

    # Severity score
    severity = (
        len(report.crashes) * 10
        + len(report.hangs) * 5
        + len(report.violations) * 2
    )
    print()
    if severity == 0:
        print(f"  🛡️  SEVERITY: NONE — Tool survived all attacks")
    elif severity < 20:
        print(f"  ⚠️  SEVERITY: LOW ({severity})")
    elif severity < 50:
        print(f"  🔴 SEVERITY: MEDIUM ({severity})")
    else:
        print(f"  💀 SEVERITY: HIGH ({severity})")
    print("=" * 72)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Fuzz test a Go MCP tool binary")
    parser.add_argument("binary", help="Path to the Go MCP binary")
    parser.add_argument("--label", help="Label for the tool", required=True)
    parser.add_argument("--timeout", type=float, default=5.0, help="Per-attack timeout (s)")
    parser.add_argument("--quick", action="store_true", help="Quick mode: fewer attacks")
    args = parser.parse_args()

    report = fuzz_go_tool(args.binary, args.label, timeout=args.timeout)
    print_report(report)

    # Exit with non-zero if crashes found
    if report.crashes:
        sys.exit(1)
    elif report.hangs:
        sys.exit(2)
    sys.exit(0)
