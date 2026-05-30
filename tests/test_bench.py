"""Tests for the SWE-bench harness — using DryRunRunner so no LLM or network needed."""
import json
from pathlib import Path


from sin_code_bundle.bench import (
    ArmSummary,
    BenchReport,
    DryRunRunner,
    Task,
    TaskResult,
    _summarize,
    format_report,
    load_tasks_jsonl,
)


SAMPLE_TASK = Task(
    instance_id="test/repo__001",
    repo="test/repo",
    base_commit="abc123",
    problem_statement="Fix the bug.",
    fail_to_pass=["tests/test_bug.py::test_fix"],
)


def test_dry_runner_returns_empty_diff():
    runner = DryRunRunner()
    diff = runner.run(SAMPLE_TASK, Path("."), sin_enabled=False)
    assert diff == ""


def test_summarize_zero_resolved():
    results = [
        TaskResult(
            instance_id="x",
            arm="control",
            resolved=False,
            duration_s=1.0,
            patch_applied=False,
            fail_to_pass_passed=0,
            fail_to_pass_total=1,
        )
    ]
    s = _summarize("control", results)
    assert s.resolved == 0
    assert s.resolved_rate == 0.0


def test_summarize_all_resolved():
    results = [
        TaskResult(
            instance_id="x",
            arm="sin",
            resolved=True,
            duration_s=2.5,
            patch_applied=True,
            fail_to_pass_passed=1,
            fail_to_pass_total=1,
        )
    ]
    s = _summarize("sin", results)
    assert s.resolved == 1
    assert s.resolved_rate == 1.0


def test_format_report_positive_delta():
    arms = {
        "control": ArmSummary("control", 5, 1, 0.2, 10.0),
        "sin": ArmSummary("sin", 5, 3, 0.6, 12.0),
    }
    report = BenchReport(
        arms=arms,
        delta_resolved_rate=0.4,
        per_task=[],
        started_at="2026-01-01T00:00:00",
        finished_at="2026-01-01T01:00:00",
    )
    text = format_report(report)
    assert "+40.0 pp" in text
    assert "control" in text
    assert "sin" in text


def test_report_to_json():
    arms = {
        "control": ArmSummary("control", 1, 0, 0.0, 5.0),
        "sin": ArmSummary("sin", 1, 1, 1.0, 6.0),
    }
    report = BenchReport(
        arms=arms,
        delta_resolved_rate=1.0,
        per_task=[],
        started_at="2026-01-01T00:00:00",
        finished_at="2026-01-01T01:00:00",
    )
    data = json.loads(report.to_json())
    assert data["delta_resolved_rate"] == 1.0
    assert "control" in data["arms"]


def test_load_tasks_jsonl(tmp_path: Path):
    lines = [
        json.dumps(
            {
                "instance_id": "repo__1",
                "repo": "org/repo",
                "base_commit": "deadbeef",
                "problem_statement": "Fix it.",
                "FAIL_TO_PASS": ["tests/test_a.py"],
                "PASS_TO_PASS": [],
            }
        )
    ]
    f = tmp_path / "tasks.jsonl"
    f.write_text("\n".join(lines), encoding="utf-8")
    tasks = load_tasks_jsonl(f, limit=10)
    assert len(tasks) == 1
    assert tasks[0].instance_id == "repo__1"


def test_load_tasks_jsonl_limit(tmp_path: Path):
    lines = [
        json.dumps(
            {
                "instance_id": f"repo__{i}",
                "repo": "org/repo",
                "base_commit": "abc",
                "problem_statement": "Fix.",
                "FAIL_TO_PASS": [],
                "PASS_TO_PASS": [],
            }
        )
        for i in range(10)
    ]
    f = tmp_path / "tasks.jsonl"
    f.write_text("\n".join(lines), encoding="utf-8")
    tasks = load_tasks_jsonl(f, limit=3)
    assert len(tasks) == 3
