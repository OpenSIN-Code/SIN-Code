# SPDX-License-Identifier: MIT
"""Tests for sin code unified hub.

Docs: test_sin_code.doc.md
"""
import subprocess
import pytest

def _run(args, timeout=30):
    return subprocess.run(args, capture_output=True, text=True, timeout=timeout)

def test_sin_help():
    r = _run(["sin", "--help"])
    assert r.returncode == 0
    assert "code" in r.stdout

def test_sin_code_help():
    r = _run(["sin", "code", "--help"])
    assert r.returncode == 0

def test_sin_code_codocs():
    r = _run(["sin", "code", "codocs", "--root", "/Users/jeremy/dev/SIN-Code-Bundle"])
    assert r.returncode in (0, 1, 2)

def test_sin_code_debt():
    r = _run(["sin", "code", "debt", "--root", "/Users/jeremy/dev/SIN-Code-Bundle"])
    assert r.returncode in (0, 1, 2)

def test_sin_code_preflight():
    r = _run(["sin", "code", "preflight"], timeout=120)
    assert r.returncode in (0, 1, 2)

def test_sin_sckg_help():
    r = _run(["sin", "sckg", "--help"])
    assert r.returncode == 0
