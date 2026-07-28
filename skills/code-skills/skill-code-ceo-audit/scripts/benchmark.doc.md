# benchmark.sh

## What it does

Measures CEO Audit performance **per axis** and reports timing in
human-readable form. Optionally saves a JSON baseline to
`/tmp/ceo-audit-baseline.json` so future runs can detect regressions
via `--compare`.

## When to use

- Before merging changes that might slow the audit (new axis, regex
  patterns, dependency installs)
- After upgrading `scout`/`discover`/etc to confirm there's no perf
  regression
- When onboarding a new repo type and you want to set expectations
  ("this repo's full audit takes ~2 min on this hardware")

## How it differs from `audit.sh`

`audit.sh` measures **wall time** of the whole pipeline. `benchmark.sh`
measures **per-axis time** by invoking each axis script directly and
isolating the elapsed seconds for each. The two are complementary:
audit.sh tells you the user experience, benchmark.sh tells you which
axis to optimize.

## Workflow

```bash
# 1. Save baseline on a known-good machine
bash scripts/benchmark.sh ~/dev/SIN-Code --profile=FULL --save

# 2. After upgrading scout, see if anything regressed
bash scripts/benchmark.sh ~/dev/SIN-Code --profile=FULL --compare

# 3. Median over 3 runs for noisy CI environments
bash scripts/benchmark.sh ~/dev/SIN-Code --rounds=3
```

## Caveats

- A failing axis (e.g. `set -euo pipefail` + `grep` on empty input)
  produces a WARN, and the time is recorded as 0.0. Check the
  per-axis log in `/tmp/ceo-bench.*/findings/axis-<name>.log` for details.
- The script runs axes **sequentially** to isolate per-axis time. The
  real `audit.sh` runs them in parallel via `&` + `wait`, so its
  wall time will be much lower than the sum of axis times.
- `awk` prints decimal points only when `LC_ALL=C` (the script sets
  this automatically so non-en_US locales work too).

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Benchmark ran (regardless of grade) |
| 1 | Repo not found or invalid profile |
| 2 | `--compare` requested but no baseline exists |

## See also

- `audit.sh` — main entry, parallel fan-out
- `validate-install.sh` — verify toolchain is correct
- `install-skill.sh` — link the skill into the opencode runtime
