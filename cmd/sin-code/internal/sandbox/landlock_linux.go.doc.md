# landlock_linux.go

Linux-specific implementation of the SIN-Code command sandbox using raw Landlock syscalls.

## What this file does

Applies filesystem and network restrictions to the current process via the Linux Landlock LSM. It builds a ruleset from the caller's read-only and read-write paths, then calls `landlock_restrict_self`. If the kernel does not support Landlock, the syscalls fail and the caller degrades gracefully.

## Dependencies

- `golang.org/x/sys/unix` for syscall numbers and Landlock structs.
- No dependency on `github.com/landlock-lsm/go-landlock` by design.

## Related files

- `sandbox.go` — cross-platform `Policy`/`Command` API and `existing()` helper.
- `sandbox_linux.go` — Linux `platformCommand` shim and `ApplyAndExec` entry point.
- `exec_linux.go` — dispatches to `applyLandlockImpl` and handles net rule errors.

## Important values

- Syscall numbers come from `unix.SYS_LANDLOCK_*` (provided by `golang.org/x/sys/unix`).
- Access flags follow the Landlock ABI v5+ bit definitions.
- Rules are applied independently; a single unsupported path is skipped, not fatal.

## Caveats

- Landlock only restricts the current OS thread unless the kernel supports `LANDLOCK_CREATE_RULESET_VERSION` with TSYNC. The current implementation uses `unix.Syscall`, which restricts the calling thread; this matches the original design.
- Network restrictions require kernel >= 6.2 (ABI v4). On older kernels they are skipped with a warning.
