# lib/add_finding.py

Appends a single finding record to an axis JSON file (the per-axis
output produced by `axis_*.sh`). Idempotent: running it twice with
the same `gate_id` does not duplicate the gate entry, but the finding
itself IS appended both times (intentional — multiple findings per
gate are valid).

## Dependencies

- stdlib: `json`, `sys`, `pathlib`

## Touched by

- `axis_*.sh` (security, performance, quality, testing, deps, docs,
  architecture, compliance) — each axis script invokes this once per
  detected finding

## What it does

CLI signature:

```
add_finding.py <file> <gate_id> <severity> <cwe> <title> <description> <fix>
```

1. Loads the axis JSON file, or initializes a fresh
   `{"axis": "unknown", "gates": [], "findings": []}` if missing.
2. Appends a `gates[]` entry for `gate_id` if no gate with that id
   exists yet (idempotent).
3. Appends a `findings[]` entry with the full finding payload.

## Important config

- `gate_id` — must match a known gate; unknown ids are still accepted
  but will be reported as "unknown" by `score.py`
- `severity` — one of `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, `INFO`

## Usage

```bash
python3 lib/add_finding.py \
  /tmp/run/findings/security.json \
  SEC.SSRF.001 \
  HIGH \
  CWE-918 \
  "SSRF in user-supplied URL" \
  "The /proxy endpoint forwards to user-controlled URLs without validation" \
  "Validate URL against an allowlist before fetching"
```

## Known caveats

- Findings are **never de-duplicated**; running the same `add_finding`
  call twice will produce two `findings[]` entries. The gate is
  de-duplicated by id, but findings are not.
- The script overwrites the axis file (`f.write_text(json.dumps(...))`)
  in place. Concurrent axis scripts writing to the same file will race.
- No JSON validation on the existing file — if the file is corrupt
  the script will crash. Always start with a fresh axis file.
