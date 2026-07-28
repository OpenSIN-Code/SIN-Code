# scripts/report.py

Generates the four CEO Audit report formats from the run directory:

- `report.md` — board-ready Markdown with executive summary, score
  card, top 3 risks, severity breakdown, compliance section, and
  action plan
- `report.sarif` — SARIF 2.1.0 for GitHub Code Scanning
- `report.html` — self-contained HTML, PDF-exportable
- `report.json` — full programmatic dump (only if `write_json=1`)

## Dependencies

- stdlib: `html`, `json`, `sys`, `datetime`

## Touched by

- `audit.sh` — invoked as the last step of the audit pipeline
- `hooks/post_audit.py` — reads `report.md` to open in the OS
  default viewer

## What it does

1. **`make_markdown(score, repo_name, profile)`** — fills the
   `REPORT_TEMPLATE` with the per-axis score card, top 3 risks
   (sorted by `risk_score`), severity counts, the
   `_response_obj`-style action plan placeholder, and the regression
   vs last audit.
2. **`make_sarif(score, run_dir)`** — produces a SARIF 2.1.0 doc
   where each finding is a `result` with `level` mapped from
   severity (`CRITICAL/HIGH → error`, `MEDIUM → warning`,
   `LOW → note`).
3. **`make_html(score, repo_name)`** — a single-file HTML page with
   inline CSS, no JS, no external assets (PDF-export-safe).
4. **`main()`** — loads `score.json` + every `findings/<axis>.json`,
   builds the action plan from `action_plan.json`, writes all four
   report files into the run dir.

## Important config

- `REPORT_TEMPLATE` — single source of truth for the Markdown layout;
  placeholders are `{{AXIS_FINDINGS}}`, `{{ACTION_PLAN}}`,
  `{{ALL_FINDINGS}}`, replaced by `main()`.
- `SEVERITY_PENALTY` style is read from the score data, not
  re-derived here.
- Compliance percentages (`owasp_pct`, `top25_pct`, `gdpr_pct`,
  `soc2_pct`) are partially placeholders — the row is rendered but
  only `owasp_pct` is currently computed.

## Usage

```bash
python3 scripts/report.py /path/to/repo /tmp/run-2026-06-03 FULL 1
# → writes report.{md,sarif,html,json} into /tmp/run-2026-06-03/
```

## Known caveats

- `owasp_pct`, `top25_pct`, `gdpr_pct`, `soc2_pct` rows are
  currently hard-coded to the `owasp_pct` calc and `0` for the
  others. The template renders the row, but the values are not
  audit-quality until the individual percentages are computed.
- The Markdown report caps per-axis findings at 10 (with an
  "...and N more" note). The full list is in the `details` section
  and in `report.json`.
- SARIF output is suitable for GitHub Code Scanning but does not
  include `locations[]` arrays (no file:line in this version);
  GitHub will surface findings as "no location" until locations
  are populated.
- HTML escaping uses `html.escape()` on user-supplied content
  (titles, descriptions). Watch out for double-escaping if you
  pipe the output through another templating step.
