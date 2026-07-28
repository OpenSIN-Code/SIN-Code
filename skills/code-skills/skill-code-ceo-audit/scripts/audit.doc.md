# audit.sh

## What it does

The **main entry point** of the CEO Audit skill. Runs all 8 audit
axes in parallel, aggregates findings, scores them, and produces 4
report formats (Markdown, SARIF, JSON, HTML).

## When to use

This is the script you call to actually run an audit:

```bash
# Full audit of the current directory
bash skills/code-skills/skill-code-ceo-audit/scripts/audit.sh

# Specific repo
bash audit.sh /path/to/repo

# Security-only (~1 min)
bash audit.sh --profile=SECURITY

# Pre-release (skip perf/docs)
bash audit.sh --profile=RELEASE

# CI: fail if grade < B
bash audit.sh --grade=B
```

## What it does NOT do

- Modify the audited repo (read-only by design)
- Persist any state outside `~/ceo-audits/<repo>-ceo-audit-<timestamp>/`
- Require network access except for `harvest` → NVD/OSV (graceful
  fallback to local cache)

## How it works (5 phases)

1. **Pre-flight** — verify all 7 core SIN-Code tools are on PATH
2. **Phase 1: Recon** — `discover` + `map` in parallel to build a
   "DNA profile" of the repo
3. **Phase 2: 8 axes** — `axis_<name>.sh` scripts fanned out via
   `&` + `wait` (parallel). Each writes its own JSON to
   `findings/<name>.json`.
4. **Phase 3-4: Score** — `scripts/score.py` aggregates findings
   and produces `score.json` (grade, score, critical/high counts,
   cost-to-fix estimate)
5. **Phase 5: Report** — `scripts/report.py` generates
   `report.md`, `report.sarif`, `report.json`, `report.html`

Total wall time: 3-5 min for a typical repo, <1 min for the
SECURITY profile, ~30s for QUICK.

## Options

| Option | Default | What it does |
|--------|---------|--------------|
| `--profile=PROFILE` | FULL | FULL \| SECURITY \| RELEASE \| QUICK |
| `--grade=X` | (none) | CI mode: exit 0 only if grade ≥ X (A/B/C) |
| `--output=DIR` | `~/ceo-audits/` | Output directory |
| `--no-color` | false | Disable ANSI colors (use in CI logs) |
| `--json` | false | Also write `report.json` sidecar |
| `--help` / `-h` | — | Show help |

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Grade A or better, no CRITICAL |
| 1 | Grade B-C, OR `--grade` gate passed but other issues |
| 2 | Grade D |
| 3 | Grade F, or any CRITICAL finding |
| 4 | Audit failed (missing tool, unreadable repo) |

CI usage: `audit.sh --grade=B` returns exit 0 only on B or better.
For PR comments, also see `post_audit_pr.py`.

## Output structure

```
<output>/<repo>-ceo-audit-<timestamp>/
  ├─ report.md          Board-ready Markdown
  ├─ report.sarif       GitHub Code Scanning
  ├─ report.html        PDF-exportable
  ├─ report.json        Programmatic (with --json)
  ├─ score.json         Numeric score breakdown
  ├─ action_plan.json   ROI-ranked fixes
  └─ findings/          Raw per-axis output
      ├─ security.json
      ├─ performance.json
      ├─ ... (one per axis)
      ├─ 01-discover.json    Phase 1 recon
      └─ 02-map.json
```

## Touched by

- `templates/ceo-audit.yml` — invokes this script in CI
- `examples/integration-ci.yml` — same, with App commenter
- `scripts/benchmark.sh` — invokes each axis directly (bypasses
  the outer fan-out) to isolate per-axis time
- `tests/test_audit_end_to_end.py` — runs this script with
  `--profile=SECURITY` against a 10-file fake repo

## See also

- `validate-install.sh` — pre-flight check before running this
- `install-skill.sh` — sets up the skill so this script works
- `benchmark.sh` — performance regression detection
- `SKILL.md` — top-level docs (47 gates, 8 axes, methodology)
