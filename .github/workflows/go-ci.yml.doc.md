# go-ci.yml

Go CI for the `sin-code` binary.

## What this workflow does

- **go test**: Checks out the repo, sets up Go 1.25.11, installs `gopls`, builds `cmd/sin-code`, runs `go vet`, validates bundled skills, and runs the Go test suite. It also runs an opt-in LSP live test.
- **benchmark**: Runs only benchmarks (`-run='^$'`) in `cmd/sin-code/internal/`, captures the output, and checks that the indexed search is at least 3x faster than the full-scan baseline.

## Related files

- `cmd/sin-code/` — the main Go binary.
- `cmd/sin-code/internal/` — internal packages and tests.
- `cmd/sin-code/internal/benchmark_test.go` — comparison benchmark used by the speedup gate.

## Caveats

- The benchmark step uses `set -euo pipefail` and `-run='^$'` so that only benchmarks run and any failure in the benchmark pipeline is surfaced.
- CI runners have slower I/O than local machines; the 3x threshold is a relaxed gate compared to the 5x+ target verified locally.
