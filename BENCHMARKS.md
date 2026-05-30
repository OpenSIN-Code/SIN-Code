# SIN-Code Benchmarks

We measure one thing: **does exposing the SIN tools improve an agent's
resolved-rate?** The harness (`sin bench`) runs the same task set twice — once
with SIN tools disabled (`control`) and once enabled (`sin`) — and reports the
delta in percentage points.

## Reproduce

```bash
pip install "sin-code-bundle[bench]"

# Smoke test (no LLM cost — validates the clone/apply/test pipeline)
sin bench --runner dry --limit 5

# Full A/B on SWE-bench Lite with opencode
sin bench --runner opencode --limit 100 --out report.json
```

## Methodology

- **Dataset:** SWE-bench Lite (`princeton-nlp/SWE-bench_Lite`, test split).
- **Arms:** `control` (SIN_ENFORCE=0) vs `sin` (SIN_ENFORCE=1, MCP tools loaded).
- **Resolved:** patch applies cleanly AND all FAIL_TO_PASS tests pass.
- **Isolation:** each task runs in a fresh git clone at `base_commit`.

## Results

| Arm | Resolved | Rate | Mean time |
|-----|----------|------|-----------|
| control | *TBD* | *TBD* | *TBD* |
| sin | *TBD* | *TBD* | *TBD* |
| **delta** | | ***TBD* pp** | |

> Fill this table from `report.json` after a full run and commit the
> `report.json` alongside the version tag so results are auditable.

## Interpretation

A positive delta means the SIN tools (impact analysis, semantic diff, Oracle
verification) caused the agent to produce more correct patches. The harness is
runner-agnostic — the same JSON report can compare opencode, codex, and hermes
on identical tasks.
