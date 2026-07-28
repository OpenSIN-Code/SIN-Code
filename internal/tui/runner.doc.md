# runner.go

Subprocess runner for the `sin tui` binary.

## What this file does

Executes `sin` commands as an explicit argv list, captures stdout and stderr separately and renders both in the same
output view, and emits `runFinishedMsg` when the child process exits. The Bubbletea
model applies the output inside `Update`, so background commands never mutate UI
state directly.

## Security contract

- No system shell is started.
- Combined stdout/stderr is capped at 4 MiB and marked when truncated.
- Secret-like argv flags are redacted in the displayed command line.
- `;`, `|`, redirects, `$()`, and similar metacharacters remain literal argv bytes.
- User input supports whitespace, single/double quotes, and backslash escaping.
- Unterminated quotes and trailing escapes fail before process creation.
- Static command keys and prompted arguments are tokenized separately.

This intentionally removes the previous `sh -c` behavior. Pipelines belong in
an explicit script or a purpose-built command, not an interpolated TUI prompt.

## Usage

```go
argv := []string{"sin", "doctor"}
cmd := tea.Batch(m.spinner.Tick, runCommand(argv, m.appendStream))
```

## Limits

The child process does not receive interactive stdin and inherits the current
working directory.
