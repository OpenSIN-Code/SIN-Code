# test_audit_end_to_end.py

## What it tests

The full CEO Audit pipeline, end to end:

```
audit.sh
   ↓
axis_security.sh (and the other axis scripts, when profile≠SECURITY)
   ↓
lib/add_finding.py
   ↓
scripts/score.py
   ↓
scripts/report.py
   ↓
report.md / report.sarif / report.json / score.json
```

This is the **highest-value test in the suite** — it catches integration
breakage that unit tests would miss (e.g. a renamed environment variable
between `audit.sh` and `score.py`, a SARIF schema drift, or a missing
`mkdir -p` that causes report.md generation to silently fail).

## How it works

1. Creates a tiny synthetic repo in a `tmp_path` fixture: 10 files
   across Python, Go, TypeScript, with deliberately bad patterns
   (hardcoded API key, `subprocess shell=True`, `hashlib.md5`,
   `time.sleep` in test).
2. Invokes `audit.sh --profile=SECURITY` against the fake repo.
3. Asserts that the run produced the expected output files and that
   each one is well-formed.

## What it explicitly does NOT test

- **Specific finding counts.** The pre-existing `axis_security.sh`
  uses Go-RE2 quantifier patterns that the scout binary silently
  fails to match (e.g. `'(api[_-]?key|...)\\s*=\\s*['\\\"][A-Za-z0-9]{20,}'`).
  Per the "NEVER change behavior of existing scripts" rule, we cannot
  fix that pattern. We assert only the **schema** of the security.json
  output, not the content. A separate test for `check_security()` in
  `test_sin_tools.py` exercises the Python path with mocked scout.
- **Performance.** The test allows up to 180s; on a 10-file repo the
  actual run is ~6s.
- **CI-grade enforcement.** The test does not fail on grade; it only
  verifies the pipeline produced files. Use `audit.sh --grade=B` for
  CI.

## Skip behavior

The whole module is **skipped** when any of the 7 core SIN-Code tools
(`discover`, `map`, `grasp`, `scout`, `execute`, `harvest`,
`orchestrate`) are missing from PATH. This is the right behavior: the
test is integration-only and the developer can run unit tests without
the full toolchain installed.

To run:

```bash
cd ~/.config/opencode/skills/ceo-audit
python3 -m pytest tests/test_audit_end_to_end.py -v
```

## See also

- `test_github_app.py` — 18 unit tests for OAuth/App integration
- `test_sin_tools.py` — 17 unit tests for the SIN-Code wrapper
- `audit.sh` — the entry point under test
- `SKILL.md` — top-level docs
