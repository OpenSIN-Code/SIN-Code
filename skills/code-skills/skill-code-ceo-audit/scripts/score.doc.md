# scripts/score.py

CEO Audit scoring engine. Reads the 8 per-axis JSON files
(`findings/<axis>.json`) plus the aggregate, computes:

- per-axis score (0-100)
- weighted total (8 axes, weights must sum to 1.0)
- letter grade (`A+`/`A`/`B`/`C`/`D`/`F`)
- risk score per finding (likelihood × impact × CWE boost)
- ROI-ranked action plan
- regression detection vs the previous audit in the same output dir
- compliance mapping (OWASP ASVS coverage)

## Dependencies

- stdlib: `json`, `math`, `sys`, `collections.Counter`, `datetime`

## Touched by

- `audit.sh` — invoked after all `axis_*.sh` scripts have written
  their JSON
- `scripts/report.py` — reads the `score.json` produced here
- `hooks/post_audit.py` — reads `score.json` to summarize + record
  in SIN-Brain

## What it does

1. **`score_axis(axis_data)`** — `100 − Σ severity_penalty − 2 × skipped_gates`
   clamped to `[0, 100]`. Skips are missing assurance, not defects, but cannot
   produce a perfect score. Also returns findings with a `risk_score` attached
   (`likelihood × impact`, where `impact = 1.5` for CWE Top-25 hits).
2. **`grade(score, critical_count)`** — letter grade. **Any
   `CRITICAL` finding caps the grade at `F`** regardless of score.
3. **`detect_regressions(current, previous)`** — diffs findings
   by `(gate, title)` pair; returns new + fixed counts and the
   list of regressions.
4. **`estimate_fix_hours(finding)`** — base hours by severity
   (`CRITICAL=4h`, `HIGH=2h`, `MEDIUM=1h`, `LOW=0.5h`, `INFO=0.1h`),
   multiplied by 2.0 if the fix contains "refactor"/"split" and
   by 1.5 if it contains "add test".
5. **`compute_roi(findings)`** — `risk / max(hours, 0.1)`, sorted
   descending. Grouped findings scale effort sub-linearly by occurrence count,
   capped at 6× the single-site estimate.
6. **`main()`** — orchestrates everything, persists full per-axis findings for
   cross-run regression detection, and writes `score.json`
   + `action_plan.json` (top 20). Exits 0 on success, 1 on
   `grade_gate` failure.

## Important config

- `AXIS_WEIGHTS` — **must sum to 1.0**. Security is 0.30 (highest)
  because security flaws are game-over.
- `SEVERITY_PENALTY` — points deducted per finding, by severity.
  Tuned so one CRITICAL caps a clean axis at 75.
- `CWE_TOP25_IMPACT` — the same Top 25 as `lib/cwe.py`; bumping
  `impact` for a finding is what makes it "high ROI".
- `grade_gate` — CI mode; passes if score ≥ threshold
  (`A=85`, `B=70`, `C=55`).

## Usage

```bash
python3 scripts/score.py /path/to/repo /tmp/run-2026-06-03 B
# → writes score.json + action_plan.json
# exit 0 if grade ≥ B, else exit 1
```

## Known caveats

- Any single `CRITICAL` finding → grade `F`, no matter how many
  other 100% axes. This is intentional but controversial; a
  "warning-only" critical can be demoted by changing `critical` to
  `HIGH` in the source.
- `detect_regressions` keys on `(gate, title)`. Renaming a gate
  or rewording a title across audits will look like a "new
  finding" even when the underlying issue is unchanged.
- `estimate_fix_hours` is a **rough heuristic**, not an actual
  estimate. Use as a relative ranking signal, not a sprint plan.
- Regression detection only looks at the most recent prior run
  in the same output dir; `run_dir.parent` must contain at least
  two `<repo>-ceo-audit-*` directories.
