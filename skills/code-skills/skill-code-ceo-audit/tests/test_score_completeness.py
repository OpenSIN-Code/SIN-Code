"""Regression tests for profile-aware and fail-closed CEO Audit scoring."""

from __future__ import annotations

import importlib.util
import json
import subprocess
import sys
from pathlib import Path

import pytest

SKILL_ROOT = Path(__file__).resolve().parents[1]
SCORE_SCRIPT = SKILL_ROOT / "scripts" / "score.py"
REPORT_SCRIPT = SKILL_ROOT / "scripts" / "report.py"


def _load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _write_axis(run_dir: Path, axis: str, findings: list[dict] | None = None) -> None:
    target = run_dir / "findings" / f"{axis}.json"
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps({"axis": axis, "gates": [], "findings": findings or []}))


def _run_score(repo: Path, run_dir: Path, gate: str = "B") -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        [sys.executable, str(SCORE_SCRIPT), str(repo), str(run_dir), gate],
        text=True,
        capture_output=True,
        check=False,
    )


def test_quick_profile_normalizes_only_requested_axes(tmp_path: Path) -> None:
    repo = tmp_path / "repo"
    repo.mkdir()
    run_dir = tmp_path / "audit-run"
    _write_axis(run_dir, "security")
    _write_axis(run_dir, "quality")
    (run_dir / "run_meta.json").write_text(
        json.dumps(
            {
                "profile": "QUICK",
                "requested_axes": ["security", "quality"],
                "failed_axes": [],
                "missing_tools": [],
                "recon_failed": [],
            }
        )
    )

    result = _run_score(repo, run_dir)
    score = json.loads((run_dir / "score.json").read_text())

    assert result.returncode == 0, result.stdout + result.stderr
    assert score["audit_complete"] is True
    assert score["grade"] == "A+"
    assert score["score"] == 100.0
    assert set(score["axes"]) == {"security", "quality"}
    assert sum(score["weights"].values()) == pytest.approx(1.0)
    assert score["weights"]["security"] == pytest.approx(2 / 3)
    assert score["weights"]["quality"] == pytest.approx(1 / 3)


def test_failed_requested_axis_forces_incomplete_f_and_nonzero_exit(tmp_path: Path) -> None:
    repo = tmp_path / "repo"
    repo.mkdir()
    run_dir = tmp_path / "audit-run"
    _write_axis(run_dir, "security")
    _write_axis(run_dir, "quality")
    (run_dir / "run_meta.json").write_text(
        json.dumps(
            {
                "profile": "QUICK",
                "requested_axes": ["security", "quality"],
                "failed_axes": ["quality"],
                "missing_tools": ["scout"],
                "recon_failed": [],
            }
        )
    )

    result = _run_score(repo, run_dir)
    score = json.loads((run_dir / "score.json").read_text())

    assert result.returncode == 1
    assert score["audit_complete"] is False
    assert score["grade"] == "F"
    assert score["axes"]["quality"]["status"] == "failed"
    assert score["axes"]["quality"]["score"] == 0
    assert "requested axis execution failed: quality" in score["audit_errors"]
    assert "complete=False" in result.stdout


def test_missing_requested_axis_cannot_receive_perfect_score(tmp_path: Path) -> None:
    repo = tmp_path / "repo"
    repo.mkdir()
    run_dir = tmp_path / "audit-run"
    _write_axis(run_dir, "security")
    (run_dir / "run_meta.json").write_text(
        json.dumps(
            {
                "profile": "QUICK",
                "requested_axes": ["security", "quality"],
                "failed_axes": [],
                "missing_tools": [],
                "recon_failed": [],
            }
        )
    )

    result = _run_score(repo, run_dir)
    score = json.loads((run_dir / "score.json").read_text())

    assert result.returncode == 1
    assert score["audit_complete"] is False
    assert score["grade"] == "F"
    assert score["score"] < 100
    assert score["axes"]["quality"]["status"] == "missing"


def test_report_marks_incomplete_audit_as_non_authoritative() -> None:
    report = _load_module(REPORT_SCRIPT, "ceo_audit_report_test")
    markdown = report.make_markdown(
        {
            "audit_complete": False,
            "audit_errors": ["requested axis execution failed: quality"],
            "grade": "F",
            "score": 66.7,
            "axes": {
                "security": {"score": 100, "finding_count": 0, "status": "complete"},
                "quality": {"score": 0, "finding_count": 0, "status": "failed"},
            },
            "weights": {"security": 2 / 3, "quality": 1 / 3},
            "severity_counts": {},
            "critical": 0,
            "high": 0,
            "total_findings": 0,
            "total_fix_hours_est": 0,
            "top_3_risks": [],
            "regression": {},
            "compliance": {},
        },
        "repo",
        "QUICK",
    )

    assert "**Audit complete** | **NO**" in markdown
    assert "Execution status: INCOMPLETE" in markdown
    assert "must not be interpreted as a repository quality score" in markdown
    assert "| Quality | failed | 100% | 0 |" in markdown


def test_skipped_gates_reduce_assurance_score(tmp_path: Path) -> None:
    repo = tmp_path / "repo"
    repo.mkdir()
    run_dir = tmp_path / "audit-run"
    target = run_dir / "findings" / "security.json"
    target.parent.mkdir(parents=True)
    target.write_text(
        json.dumps(
            {
                "axis": "security",
                "gates": [
                    *[{"id": f"1.{i}", "status": "pass"} for i in range(1, 10)],
                    *[{"id": f"1.{i}", "status": "skipped"} for i in range(10, 13)],
                ],
                "findings": [],
            }
        )
    )
    (run_dir / "run_meta.json").write_text(
        json.dumps(
            {
                "profile": "SECURITY",
                "requested_axes": ["security"],
                "failed_axes": [],
                "missing_tools": [],
                "recon_failed": [],
            }
        )
    )

    result = _run_score(repo, run_dir, gate="B")
    score = json.loads((run_dir / "score.json").read_text())

    assert result.returncode == 0
    assert score["score"] == 94.0
    assert score["grade"] == "A"
    assert score["axes"]["security"]["gate_coverage"] == pytest.approx(0.75)
    assert score["axes"]["security"]["skipped_gate_count"] == 3


def test_score_persists_findings_and_detects_fixed_regression(tmp_path: Path) -> None:
    repo = tmp_path / "repo"
    repo.mkdir()

    first = tmp_path / "repo-ceo-audit-20260101-000000"
    finding = {
        "gate": "1.3",
        "severity": "LOW",
        "cwe": "CWE-78",
        "title": "Old issue",
        "description": "fixture",
        "fix": "use argv",
        "locations": ["app.py:1"],
    }
    _write_axis(first, "security", [finding])
    (first / "run_meta.json").write_text(
        json.dumps(
            {
                "profile": "SECURITY",
                "requested_axes": ["security"],
                "failed_axes": [],
                "missing_tools": [],
                "recon_failed": [],
            }
        )
    )
    first_result = _run_score(repo, first, gate="C")
    assert first_result.returncode == 0
    first_score = json.loads((first / "score.json").read_text())
    assert first_score["axes"]["security"]["findings"][0]["title"] == "Old issue"

    second = tmp_path / "repo-ceo-audit-20260102-000000"
    _write_axis(second, "security")
    (second / "run_meta.json").write_text(
        json.dumps(
            {
                "profile": "SECURITY",
                "requested_axes": ["security"],
                "failed_axes": [],
                "missing_tools": [],
                "recon_failed": [],
            }
        )
    )
    second_result = _run_score(repo, second, gate="B")
    assert second_result.returncode == 0
    second_score = json.loads((second / "score.json").read_text())
    assert second_score["regression"]["new"] == 0
    assert second_score["regression"]["fixed"] == 1
