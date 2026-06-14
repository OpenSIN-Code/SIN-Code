# `update_cmd.doc.md` — `sin update` Subcommand

Top-level orchestrator for the full SIN-Code stack update: Python pipx
packages, local Go binaries, and skills. Also provides rollback and dry-run
modes.

## What it does

- **Parses flags** for mutually exclusive `--python-only`, `--go-only`,
  `--skills-only`, plus `--check`, `--dry-run`, `--force`, `--rollback`,
  `--skip-doctor`, `--state-root`, and `--keep-snapshots`.
- **Creates a snapshot** via `BackupManager` before any mutations.
- **Writes a manifest** describing the pre-update state.
- **Runs phases**: Python (`RunPythonPhase`), Go (`RunGoPhase`) — skills
  currently share the Python phase.
- **Runs `sin-code doctor`** as a non-fatal post-update health check unless
  `--skip-doctor` is passed.
- **Prunes old snapshots** to keep at most `--keep-snapshots`.
- **Rollback mode** restores the latest snapshot's Go binaries without running
  phases.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `UpdateCmd`.
- `update_phases.go` — phase implementations invoked by `runUpdate`.
- `update_backup.go` / `update_rollback.go` / `update_manifest.go` — snapshot
  lifecycle and restore logic.
- `update_cmd_test.go` — exercises every branch of `runUpdate` via test hooks.

## Important config values & limits

- **State root**: default `~/.local/state/sin-code`, override with
  `--state-root` or `SIN_CODE_STATE_ROOT`.
- **Snapshot retention**: default 10, set with `--keep-snapshots`.
- **Update timeout**: 5 minutes hard context timeout.
- **Mutual exclusion**: `--python-only`, `--go-only`, `--skills-only` cannot be
  combined.

## Test hooks

- `runPythonPhaseFn`, `runGoPhaseFn`, `runDoctorNonFatalFn`, and
  `pruneSnapshotsFn` are package-level variables so `runUpdate` error and
  warning paths can be tested without invoking real `pipx`/`go`.

## Usage examples

```bash
sin-code update              # full update
sin-code update --check      # enumerate only
sin-code update --dry-run      # show plan, no mutations
sin-code update --rollback     # restore previous snapshot
sin-code update --python-only  # only pipx packages
```

## Known caveats

- **Phase errors are fatal**: if `RunPythonPhase` or `RunGoPhase` returns an
  error, the update aborts immediately; the snapshot is left in place for
  manual rollback.
- **Doctor is non-fatal**: a failing `doctor` run only prints a warning.
- **Prune is non-fatal**: prune errors only print a warning.
