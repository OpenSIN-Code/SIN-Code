# agents_md.py

Generates and idempotently updates the `AGENTS.md` file in a repo with a
SIN-Code-Block that tells the agent *when* to call which tool.

## Dependencies

- `pathlib.Path` (stdlib only — no third-party deps)
- `__future__.annotations` for 3.9+ type hint portability

## Touched by

- `cli.py` — exposed as the `sin agents-md` command
- `install.sh` — invoked once during bundle setup to seed the target repo

## What it does

1. **Renders** a markdown block listing SIN-Code tool triggers, between two
   HTML comment markers (`<!-- sin:start -->` / `<!-- sin:end -->`).
2. **Idempotent upsert**:
   - File missing → create with the SIN block as the body
   - File present, no block → append the block (with a single blank-line sep)
   - File present, block exists → replace the *contents* between markers only
     (everything else in the file is preserved verbatim)

## Important constants

- `START_MARKER = "<!-- sin:start -->"` — never change without bumping the
  bundle version and writing a migration tool
- `END_MARKER = "<!-- sin:end -->"` — same rule
- `_PLAYBOOK` — the canonical trigger table; keep small and imperative

## Usage

```python
from pathlib import Path
from sin_code_bundle.agents_md import upsert

print(upsert(Path("AGENTS.md")))
# → "Updated SIN-Code block in AGENTS.md"  (or "Created ...", "Appended ...")
```

## Known caveats

- The block is **English-only**; non-English AGENTS.md files get English content
  injected.
- Idempotency relies on exact marker text; do not hand-edit the markers.
- `render_block()` returns the inner content; `render_full_document()` returns
  the entire file template.
