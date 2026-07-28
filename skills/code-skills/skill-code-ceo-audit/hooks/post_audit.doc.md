# hooks/post_audit.py

Post-audit hook invoked by `audit.sh` after `score.py` and `report.py`
have finished. Summarizes the result to the terminal, opens the
Markdown report in the OS default viewer, prints a follow-up
checklist, and (if SIN-Brain is installed) records the audit in
long-term memory.

## Dependencies

- stdlib: `json`, `os`, `platform`, `subprocess`, `sys`, `pathlib`
- optional: `sin_brain` (silently skipped if not installed)

## Touched by

- `audit.sh` — last hook in the pipeline
- `~/.config/opencode/hooks/` — does NOT install itself there; the
  hook is only invoked from the CLI flow

## What it does

1. **`open_report(run_dir)`** — opens `report.md` in the OS default
  viewer (`open` on macOS, `xdg-open` on Linux, `os.startfile` on
  Windows). Silently no-ops if `report.md` is missing.
2. **`show_summary(score_path)`** — prints a one-line summary like
  `B    78.3/100  crit=0  high=2  total=14`.
3. **`record_in_sin_brain(run_dir, score)`** — if `sin_brain` is
  installed, calls `cortex.observe(category="ceo-audit", …)` so
  future audits can compare against this baseline.
4. **`suggest_follow_ups(score, run_dir)`** — prints a `  - [ ] …`
  checklist whose items depend on grade and finding counts.
5. **`main(run_dir)`** — orchestrates: validates `score.json` exists,
  prints the summary, prints the follow-ups, records in SIN-Brain,
  opens the report.

## Important config

- `suggest_follow_ups` is **grade-driven**:
  - `A+` / `A` → schedule quarterly re-audit
  - `B` / `C` → re-audit after addressing findings
  - `D` / `F` → schedule follow-up in 1-2 weeks
  - any `critical > 0` → address immediately
  - any `high > 0` → address before next release

## Usage

```bash
python3 hooks/post_audit.py /tmp/run-2026-06-03
# → prints summary, follow-up checklist, opens report.md
```

## Known caveats

- `os.startfile()` only exists on Windows; the `type: ignore` is
  needed because the type checker does not know that.
- `record_in_sin_brain` catches **all** exceptions and prints to
  stderr; a broken SIN-Brain install will not fail the audit.
- `open_report` uses `check=False` for `subprocess.run`, so a
  missing `open`/`xdg-open` is silent.
- The follow-up checklist is **text only** — it does not create
  GitHub issues or Linear tickets. Wire your own automation to
  parse `score.json` + `action_plan.json` for that.
