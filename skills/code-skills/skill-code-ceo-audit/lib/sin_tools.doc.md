# lib/sin_tools.py

Thin wrapper around the SIN-Code Go tool suite (discover, map, grasp,
scout, …). Each axis script calls one of the helpers to run a
graph-aware analysis instead of raw grep/find.

## Dependencies

- stdlib: `json`, `shutil`, `subprocess`
- external: the SIN-Code Go binaries on `PATH` (installed via
  `SIN-Code/install.sh`)

## Touched by

- `axis_security.sh` / `axis_quality.sh` / `axis_performance.sh` /
  `axis_architecture.sh` — every axis that needs structural
  context

## What it does

Exposes:

- **`call_sin_tool(tool, args, timeout=60)`** — the low-level
  JSON-RPC invocation. Returns `{"error": ...}` on failure (no
  exception) so axes can `grep` for "error" without try/except.
- **`extract_text(response)`** — pulls the `result.content[0].text`
  field from a JSON-RPC response.
- **`count_matches(text)`** — counts `Match`/`match` lines in a
  scout/grep-style response.
- **`discover(path, pattern, max_results)`** — quick wrapper over
  `discover`.
- **`scout(path, query, search_type, max_results)`** — quick wrapper
  over `scout`.
- **`map_arch(path)`** — quick wrapper over `map`.
- **`grasp(file)`** — quick wrapper over `grasp`.

Per-axis high-level checks (added v0.3.0). Each returns
`{"findings": [...], "error": "..." (only on failure)}` and produces
finding dicts in the same shape that `scripts/add_finding.py` writes
to the per-axis JSON file:

- **`check_security(repo, max_findings=50)`** — runs 11 of the 12
  security gates (CWE-798, 89, 78, 22, 918, 502, 327, 338, 1333, ASVS-V3.5, 601).
  Bandit-only gate (1.8) is left to the axis shell script.
- **`check_performance(repo, max_findings=50)`** — runs 5 of the 6
  performance gates (nested loops, giant slices, unbounded caches,
  regex-per-call, sync I/O).
- **`check_quality(repo, max_findings=50)`** — runs the file-size
  and naming-convention quality gates.
- **`check_testing(repo, max_findings=50)`** — runs the test-framework
  and `time.sleep` gates.
- **`check_deps(repo, max_findings=50)`** — runs the unpinned-version
  gate (real CVE checking needs harvest → NVD/OSV, left to the axis).
- **`check_docs(repo, max_findings=50)`** — runs README + CHANGELOG
  presence checks.
- **`check_architecture(repo, max_findings=50)`** — runs the
  map-empty and god-module gates.
- **`check_compliance(repo, max_findings=50)`** — runs the LICENSE +
  SECURITY.md presence checks.
- **`check_axis(axis, repo)`** — dispatch by axis name. Returns
  `{"error": "unknown axis: ...", "findings": []}` for invalid input.
- **`AXIS_CHECKS`** — dict mapping axis name → `check_<axis>()` fn,
  used by the dispatcher and convenient for plugins.

## Important config

- `timeout = 60` — default per-call timeout; bump to 300+ for large
  repos
- The Go binaries are invoked as `<tool> --mcp` (JSON-RPC over stdio,
  not HTTP)

## Usage

```python
from lib.sin_tools import discover, scout, map_arch, grasp

files = discover(".", "**/*.py", max_results=200)
hits = scout(".", "TODO", search_type="regex")
arch = map_arch(".")
summary = grasp("backend/pool_manager.py")
```

## Known caveats

- Errors come back as `dict` (not exceptions); callers must check
  `if "error" in response:` explicitly. The convention is to
  short-circuit and write the error to the axis JSON.
- `count_matches()` is line-based and case-sensitive on `Match` /
  case-insensitive on `match`. It will miscount if a tool reformats
  its output to a single line.
- The wrapper assumes each Go binary accepts `--mcp` and reads a
  single JSON-RPC payload from stdin. Older or newer versions may
  need a translation shim.
