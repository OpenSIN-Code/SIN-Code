# copy.doc.md

Tiny clipboard writer — shells out to `pbcopy`.

## What this file does

Provides `CopyToClipboard(text string) error` — pipes `text` into the
macOS `pbcopy` binary. Used by `app.go` when the user presses `y` in
`StateOutput` to grab the visible command output.

## Dependencies

Standard library only (`os/exec`, `io`). No `github.com/atotto/clipboard`
or similar third-party deps; keeps the binary small and the failure mode
trivial (`exec.ErrNotFound` on Linux/Windows).

## Magic constants

```go
const clipboardBin = "pbcopy"  // macOS-only; see Caveats for Linux
```

## Design decisions

- **`exec.Command` instead of a library** — we already shell out for
  every TUI action via `runner.go`, so `exec` is on the import path
  regardless. Saves a transitive dep.
- **Errors are returned verbatim** — the caller decides how to surface
  them. Today `app.go` flashes the error in the toast so the user gets
  immediate feedback when `pbcopy` is missing.
- **`stdin.Close()` before `cmd.Wait()`** — `pbcopy` reads until EOF;
  closing the pipe signals end-of-input. Forgetting this is a classic
  cause of hangs.

## Example

```go
if err := CopyToClipboard("hello"); err != nil {
    log.Printf("clipboard: %v", err)
}
```

## Caveats

- **macOS only.** On Linux/Windows this returns `exec.ErrNotFound`. If
  we ever ship cross-platform, switch to a `switch runtime.GOOS` and
  fall back to `wl-copy` / `xclip` / `clip.exe`.
- **No size limit.** A 10 MB output will be 10 MB in the clipboard;
  `pbcopy` accepts it but the system pasteboard may reject huge
  payloads. Acceptable for typical CLI output.
- **No tests.** Calling `pbcopy` from a test would either pollute the
  developer's clipboard or fail in CI. The function is one `exec.Run`
  call — exercise it via the TUI in interactive QA.
