# `auto-coverage` hooklife hook

## Purpose

Enqueue a coverage-driven test-generation request every time a `Write` or
`Edit` tool touches a `.go` file inside the SIN-Code project. The request is
written to `.sin-code/coverage-requests/<import-path>.json` and can be
consumed by `sin-code cover generate` or by an autonomous worker.

## Trigger

- Hook phase: `PostToolUse`
- Tool: `Write` or `Edit`
- File extension: `.go`

## Output

```json
{
  "package": "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife",
  "file": "coverage.go",
  "hint": "auto-generated request after Write"
}
```

## Configuration

The hook is controlled by the global variable
`internal/learning.AutoCoverage`. It is `true` by default in the SIN-Code
repo so every `.go` edit produces a request. Set it to `false` to disable
queueing.

## Why not run `go test` synchronously?

Running the full coverage scan after every file write would be too slow
inside the interactive agent loop. The hook only writes a tiny request file;
`sin-code cover generate` or a background worker turns the request into tests.

## Testability

All filesystem operations are swappable via package-level hooks so the 100%
coverage test suite can exercise error paths without touching the real disk.
