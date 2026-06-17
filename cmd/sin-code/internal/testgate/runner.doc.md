# `testgate` — Quality Gate Pipeline

## Purpose

Run a configurable pipeline of build/test/security checks against the
workspace and return a structured, pass/fail report. The pipeline is
used by the `sin_quality_gate` chat tool and can be wired into the
verify gate as a `verify.pre` runner.

## Pipeline Steps

| Step | Tool | Required | Failure behaviour |
|------|------|----------|-------------------|
| `build` | `go build ./...` | yes | fails gate |
| `vet` | `go vet ./...` | yes | fails gate |
| `test` | `go test ./... -race -cover` | yes | fails gate |
| `staticcheck` | `staticcheck ./...` | no | skipped if not on PATH |
| `gosec` | `gosec ./...` | no | skipped if not on PATH |
| `govulncheck` | `govulncheck ./...` | no | skipped if not on PATH |

## Configuration

- `Timeout`: total pipeline timeout (default 5m).
- `CoverageThreshold`: minimum coverage percent; 0 disables the check.
- `Steps`: subset of steps to run; empty means all.
- `Race`: enable `-race` in the test step (default true).

## Output

The `Report` struct contains a `status` field (`PASS`/`FAIL`), per-step
output, and a coverage line. When the chat tool is invoked with
`json=true`, the report is returned as indented JSON.

## Testability

Both `CommandRunner` and `LookPath` are swappable so tests can exercise
success, failure, and missing optional tools without touching the real
filesystem or PATH.
