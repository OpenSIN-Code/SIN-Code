"""Regression tests for the evidence-focused CEO quality scanner."""

from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

SCRIPT = Path(__file__).parents[1] / "scripts" / "quality_scan.py"
SPEC = importlib.util.spec_from_file_location("ceo_quality_scan", SCRIPT)
assert SPEC and SPEC.loader
quality_scan = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = quality_scan
SPEC.loader.exec_module(quality_scan)


def finding_gates(result: dict) -> set[str]:
    return {finding["gate"] for finding in result["findings"]}


def test_python_complexity_and_function_size_are_located(tmp_path: Path) -> None:
    branches = "\n".join(f"    if x == {i}:\n        x += 1" for i in range(17))
    padding = "\n".join("    x += 1" for _ in range(130))
    (tmp_path / "app.py").write_text(f"def hardThing(x):\n{branches}\n{padding}\n    return x\n")
    result = quality_scan.scan(tmp_path)
    assert {"3.1", "3.2", "3.6"} <= finding_gates(result)
    locations = {loc for finding in result["findings"] for loc in finding["locations"]}
    assert "app.py:1" in locations


def test_excludes_tests_docs_and_generated_files(tmp_path: Path) -> None:
    (tmp_path / "tests").mkdir()
    (tmp_path / "tests" / "test_huge.py").write_text("x = 1\n" * 2000)
    (tmp_path / "docs").mkdir()
    (tmp_path / "docs" / "huge.py").write_text("x = 1\n" * 2000)
    (tmp_path / "thing_generated.go").write_text("package x\n" + "var x = 1\n" * 2000)
    result = quality_scan.scan(tmp_path)
    assert result["findings"] == []


def test_large_production_file_has_exact_location(tmp_path: Path) -> None:
    (tmp_path / "large.ts").write_text("const x = 1;\n" * 1201)
    result = quality_scan.scan(tmp_path)
    finding = next(item for item in result["findings"] if item["gate"] == "3.3")
    assert finding["locations"] == ["large.ts:1"]


def test_duplicate_and_dead_code_gates_are_skipped(tmp_path: Path) -> None:
    result = quality_scan.scan(tmp_path)
    gates = {gate["id"]: gate for gate in result["gates"]}
    for gate_id in ("3.4", "3.5"):
        assert gates[gate_id]["status"] == "skipped"
        assert gates[gate_id]["reason"]


def test_go_function_metrics(tmp_path: Path) -> None:
    conditions = "\n".join(f"if x == {i} {{ x++ }}" for i in range(17))
    (tmp_path / "main.go").write_text(
        "package main\nfunc complex(x int) int {\n" + conditions + "\nreturn x\n}\n"
    )
    result = quality_scan.scan(tmp_path)
    assert "3.1" in finding_gates(result)
