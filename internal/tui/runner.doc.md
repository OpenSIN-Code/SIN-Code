# runner.doc.md

Subprocess runner for the `sin tui` binary.

## What this file does

Executes `sin <args>` in a subprocess via `sh -c`, streams stdout/stderr
line-by-line into the TUI viewport, and fires `runFinishedMsg` on exit.

## Dependencies

- `app.go` — `startCommand` builds the shell string and calls `runCommand`
- `app.go` — `Update` receives `runFinishedMsg` to transition to `StateOutput`

## Important values & limits

- `runCommand` uses `sh -c` so complex pipe expressions work.
- `scanLines` splits on `\n` so each line appears as it's emitted.
- `stream(line)` callback is `Model.appendStream` which appends to the
  output builder and refreshes the viewport.

## Design decisions

- `sh -c` wrapping lets power users pass pipe chains through the prompt.
- Stderr is merged into Stdout so error messages appear inline.
- `runFinishedMsg` carries `err` and `elapsed` for status line display.

## Usage

```go
cmd := tea.Batch(m.spinner.Tick, runCommand("sin doctor", m.appendStream))
```

## Caveats

- No stdin forwarding — the subprocess cannot accept interactive input.
- No working-directory override — runs in whatever `os.Getwd()` returns.
