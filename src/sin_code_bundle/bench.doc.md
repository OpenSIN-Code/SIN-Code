# bench.py

SWE-bench-style A/B evaluation harness that measures whether the SIN-Code
tools actually improve an agent's resolved-rate. Runs each task under
two arms (control / sin) in isolated git worktrees.

## Dependencies

- stdlib: `subprocess`, `tempfile`, `json`, `statistics`, `time`, `dataclasses`
- optional: `datasets` (only for `load_swebench_lite()`)

## Touched by

- `cli.py` — exposed as the `sin bench` command
- `SINator-v0/.../bench_runner.py` — non-test consumer of the `run_benchmark()`
  public API

## What it does

1. **Task loading** — either from a local JSONL file (SWE-bench field names)
   or via `datasets.load_dataset("princeton-nlp/SWE-bench_Lite")`.
2. **Worktree preparation** — clones the target repo at `base_commit` into
   a temp dir, runs any `setup_cmds`.
3. **Agent run** — a pluggable `AgentRunner` (e.g. `CommandRunner`) produces
   a unified diff. `DryRunRunner` is provided for cost-free smoke tests.
4. **Patch application** — `git apply --whitespace=nowarn` against the worktree.
5. **Test execution** — runs `test_cmd` for every `fail_to_pass` test id.
6. **Reporting** — `BenchReport` carries per-task results, per-arm summaries,
   and the SIN delta (resolved_rate(sin) − resolved_rate(control)).

## Important config

- `MAX_RETRIES` / `_timeout_s` — controlled per-runner; default 1800s
- `fail_to_pass` — SWE-bench convention: tests that must transition FAIL→PASS
  for the task to count as "resolved"
- `DEFAULT_TIMEOUT = 1800` for `_prepare_worktree` and `_run_named_tests`

## Usage

```python
from pathlib import Path
from sin_code_bundle.bench import (
    Task, DryRunRunner, run_benchmark, load_swebench_lite, format_report
)

tasks = load_swebench_lite(limit=20)
report = run_benchmark(tasks, runner=DryRunRunner())
print(format_report(report))
```

## Known caveats

- `CommandRunner` captures *all* stdout/stderr to a temp file; very long
  agent runs can fill the temp volume.
- `_apply_patch` returns False for empty diffs — this is intentional and
  counts as "not resolved" rather than as an error.
- The harness is runner-agnostic but only `DryRunRunner` and `CommandRunner`
  are bundled; bring your own `AgentRunner` for opencode/codex/hermes.
