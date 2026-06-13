# `update_phases.doc.md` — Update Phase Orchestration

Executes the three update phases: Python (pipx), Go (rebuild), Skills (pipx).

## Phase: Python
- Upgrades all packages in `AllPythonPackages` via `pipx upgrade <pkg>`.
- 60-second timeout per package via context.
- Idempotent: `pipx upgrade` on already-latest package is a no-op.
- In `--check` / `--dry-run` mode: enumerates packages without executing upgrades.
- Also discovers `sin-gsd-*` packages via `pipx list --json` and upgrades them.

## Phase: Go
- Rebuilds 7 standalone tools from local source repos under `$SIN_CODE_REPOS_DIR`
  (default `~/dev/`).
- Uses `git describe --tags --dirty` for version injection via `-ldflags`.
- Builds with `CGO_ENABLED=0` (Mandate M2).
- Post-build verification: runs `<binary> --version` and checks output contains
  the expected version string.
- Skips tools whose repo dir is missing (prints warning to stderr).
- Output path: `$SIN_CODE_BIN_DIR/<tool>` (default `~/.local/bin/<tool>`).

## Phase: Skills
- Currently delegates to Python phase (same pipx upgrade mechanism).
- All sin-* packages are both skills and pipx packages.

## Skip semantics
- `--check` / `--dry-run` → enumerate only, no mutations.
- `--python-only` → only Python phase runs.
- `--go-only` → only Go phase runs.
- `--skills-only` → skills phase = Python phase.

## Known caveats
- **pipx CLI surface is the only contract** — no Go SDK exists for pipx.
- **Go build from source** requires repos checked out at `$SIN_CODE_REPOS_DIR`.
  If repos are missing, the tool is silently skipped.
