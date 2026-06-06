# update.py

Powers the `sin update` CLI command. Probes installed versions, pulls
the Python bundle via `pipx upgrade`, then rebuilds every Go tool repo
under `~/dev/SIN-Code-*-Tool/` that exposes a `cmd/<name>/main.go`.

## Dependencies

- stdlib only (`subprocess`, `pathlib`, `dataclasses`, `json`)
- external: `pipx` (for the Python step), `git` + `go` (for the Go step)

## Touched by

- `cli.py` — the `update` `@app.command()` block dispatches into `run_update()`
- Tests under `tests/test_update.py` exercise `discover_go_tools`,
  `update_go_tool` (check + real), `update_python` (check + missing pipx),
  and `render_table` (empty + populated).

## What it does

1. **Discover Go tools.** Globs `~/dev/SIN-Code-*-Tool/` and for each repo
   picks the first subdirectory of `cmd/` whose `main.go` exists. Maps
   that name to `~/.local/bin/<name>` (matches the layout installed by
   `./install.sh`).
2. **Update Python.** Runs `pipx upgrade sin-code-bundle` and reports
   old → new version by re-reading `pipx list --json`.
3. **Update each Go tool.**
   - Skips the `git pull` when the repo is in detached HEAD (tags, shallow clones).
   - Otherwise: `git pull --ff-only` then
     `go build -ldflags "-X main.Version=$(git describe --tags --always)" -o ~/.local/bin/<name> ./cmd/<name>`.
4. **Render.** Returns a list of `UpdateResult` dataclasses that the CLI
   formats as a fixed-width table.

## Important constants

- `DEFAULT_DEV_DIR = ~/dev` — where Go tool source repos live
- `DEFAULT_BIN_DIR = ~/.local/bin` — where built binaries are installed
- `DEFAULT_PIPX_PKG = "sin-code-bundle"` — pipx package name
- `GIT_VERSION_FALLBACK = "unknown"` — used when neither tags nor HEAD describe work

## Usage (programmatic)

```python
from sin_code_bundle.update import run_update, render_table

# Dry run
print(render_table(run_update(check=True)))

# Real update
print(render_table(run_update(core=True, go=True)))
```

## --check semantics

In `--check` mode every step returns a `UpdateResult` with
`status="would-update"` (or `"would-skip"`) **without mutating anything**:
no `pipx upgrade`, no `git pull`, no `go build`. The CLI uses the same
`run_update` function — `--check` is just a flag.

## Known caveats

- `git pull --ff-only` will fail loudly (status="failed") when the local
  branch has diverged from origin. Run `git pull --rebase` manually first.
- The version probe for the Python package depends on `pipx list --json`
  output shape. Older pipx (< 1.0) returns a different structure and the
  version will come back as `""`. The step still completes; the table
  just shows `-` for the unknown column.
- Go tool version is read from `<binary> --version` (first line). Tools
  that don't implement `--version` will show `<binary path>` as the
  version — that's the binary's mtime hash, not a real version.
