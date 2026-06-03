# safety.py

Hardened subprocess + input-sanitization helpers shared by all subsystems.

## Dependencies

- stdlib: `subprocess`, `pathlib`, `typing`

## Touched by

- All other SIN-Code modules that spawn subprocesses (`bench.py`,
  `gitnexus.py`, `markitdown.py`, `rtk.py`).

## What it does

1. **`run_checked(cmd, cwd, timeout, allow_shell=False)`** — runs a
   subprocess with a **mandatory timeout** (default 600s) and **shell
   off by default**. Refuses non-list `cmd` unless `allow_shell=True`.
   Returns a `CompletedProcess`; never raises on non-zero exit (callers
   inspect `.returncode`).
2. **`sanitize_prompt(text, max_len=8000)`** — neutralizes obvious
   prompt-injection markers in untrusted task text:
   - hard-truncates to `max_len` chars (adds `...[truncated]`)
   - replaces lines starting with `system:`, `developer:`,
     `ignore previous`, `you are now` with
     `[redacted suspicious instruction]`

## Important config

- `DEFAULT_TIMEOUT = 600` — never run unbounded; 10 minutes is the
  upper bound for any single subprocess in the bundle

## Usage

```python
from pathlib import Path
from sin_code_bundle.safety import run_checked, sanitize_prompt

proc = run_checked(["git", "status"], cwd=Path("."))
if proc.returncode == 0:
    print(proc.stdout)

clean = sanitize_prompt(open("task.txt").read())
```

## Known caveats

- `sanitize_prompt` is a **best-effort** filter, not a security boundary.
  Sophisticated injection attempts (e.g. zero-width chars, base64
  payloads) will pass through unchanged.
- `run_checked` always sets `check=False`; callers must explicitly
  raise on non-zero exit if they want fail-fast behavior.
- `allow_shell=True` opens the full shell-injection surface; only use
  it for commands that genuinely need shell features (pipes, globs)
  and where the command string is from a trusted source.
