import tempfile
from pathlib import Path

from sin_code_oracle.eval_harness import EvalHarness, EvalTask
from sin_code_oracle.execution import ExecutionOracle
from sin_code_oracle.oracle import Confidence, VerificationOracle
from sin_code_oracle.trace_diff import TraceDiffer


def test_execution_command_success():
    oracle = ExecutionOracle(root=".")
    res = oracle.run_command("python -c \"print('ok')\"")
    assert res.success
    assert res.exit_code == 0
    assert "ok" in res.stdout


def test_execution_command_failure():
    oracle = ExecutionOracle(root=".")
    res = oracle.run_command('python -c "import sys; sys.exit(3)"')
    assert not res.success
    assert res.exit_code == 3


def test_pytest_summary_parsing():
    metrics = ExecutionOracle._parse_pytest_summary("5 passed, 1 failed, 2 skipped in 0.34s")
    assert metrics["passed"] == 5
    assert metrics["failed"] == 1
    assert metrics["skipped"] == 2


def test_trace_diff_detects_behavior_change():
    differ = TraceDiffer(root=".")
    before = differ.capture("python -c \"print('hello')\"")
    after_same = differ.capture("python -c \"print('hello')\"")
    after_diff = differ.capture("python -c \"print('goodbye')\"")
    assert not differ.diff(before, after_same).changed
    assert differ.diff(before, after_diff).changed


def test_trace_diff_normalizes_timestamps():
    differ = TraceDiffer(root=".")
    a = differ.capture("python -c \"print('2024-01-01T10:00:00Z done')\"")
    b = differ.capture("python -c \"print('2025-06-30T23:59:59Z done')\"")
    # Timestamps are normalized, so these are behaviorally identical.
    assert not differ.diff(a, b).changed


def test_oracle_unverified_when_no_signal():
    oracle = VerificationOracle(root=".")
    verdict = oracle.verify(run_diagnostics=False)
    assert verdict.verified is False
    assert verdict.passed is False
    assert verdict.confidence == Confidence.LOW


def test_oracle_passes_on_green_test():
    oracle = VerificationOracle(root=".")
    verdict = oracle.verify(
        test_command='python -c "assert 1 + 1 == 2"',
        run_diagnostics=False,
    )
    assert verdict.verified is True
    assert verdict.passed is True


def test_oracle_fails_on_red_test():
    oracle = VerificationOracle(root=".")
    verdict = oracle.verify(
        test_command='python -c "assert False"',
        run_diagnostics=False,
    )
    assert verdict.passed is False


def test_eval_harness_resolves_passing_task():
    with tempfile.TemporaryDirectory() as seed:
        Path(seed, "marker.txt").write_text("seed")
        task = EvalTask(
            id="t1",
            workspace=seed,
            verify_commands=["python -c \"open('marker.txt')\""],
        )
        harness = EvalHarness(config_label="test")

        def agent(ws, t):
            # Agent does nothing; the seed already satisfies the verifier.
            return None

        report = harness.run_suite([task], agent)
        assert report.total == 1
        assert report.resolved == 1
        assert report.resolved_rate == 1.0


def test_eval_harness_agent_can_solve():
    with tempfile.TemporaryDirectory() as seed:
        task = EvalTask(
            id="t2",
            workspace=seed,
            verify_commands=["python -c \"open('answer.txt')\""],
        )
        harness = EvalHarness()

        def agent(ws, t):
            Path(ws, "answer.txt").write_text("42")

        report = harness.run_suite([task], agent)
        assert report.resolved == 1
