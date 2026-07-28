# test_sin_tools.py

## What it tests

The Python wrapper around the SIN-Code tool suite (`lib/sin_tools.py`),
both the original low-level API (`call_sin_tool`, `extract_text`,
`count_matches`, `discover`, `scout`, `map_arch`, `grasp`) and the
new per-axis `check_<axis>()` methods added in v0.3.0.

## How tests stay hermetic

Every test that exercises `check_*()` mocks `call_sin_tool` /
`discover` / `scout` / `map_arch` directly with `unittest.mock.patch`.
No real SIN-Code binaries are required to run this suite — the
end-to-end integration with the real tools is covered by
`test_audit_end_to_end.py` (which gracefully skips when the tools
are absent).

## Coverage

- `call_sin_tool` missing-binary path
- `extract_text` error path + happy path
- `count_matches` case-insensitive counting
- `check_security` / `check_performance` / `check_quality` / `check_testing` / `check_deps` / `check_docs` / `check_architecture` / `check_compliance` shape + dispatch
- `check_axis` dispatch table covers all 8 axes
- `check_axis` unknown-axis error

## Running

```bash
cd ~/.config/opencode/skills/ceo-audit
python3 -m pytest tests/test_sin_tools.py -v
```

## See also

- `test_github_app.py` — 18 tests for OAuth/App integration
- `test_audit_end_to_end.py` — full-pipeline test with real SIN-Code tools
- `lib/sin_tools.py` — the module under test
- `SKILL.md` — top-level docs
