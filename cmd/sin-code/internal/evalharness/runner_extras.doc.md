# evalharness/runner_extras.go — CompileAndRun helpers

This file holds the implementation details for the `CompileAndRun`
scorer: fenced code-block extraction, per-language compile, and
sandboxed self-check execution.

## `extractCodeBlock(s string) string`

Returns the first Markdown fenced code block (` ```...``` `) in `s`,
stripping the info string (e.g. `python`) and surrounding whitespace.
If no block is found, returns `""`.

## Compile step

Each supported language has a dedicated `compile*` method that writes the
extracted code to a temporary file and runs the language's syntax checker:

| Language | Compile command |
|---|---|
| `python` | `python3 -m py_compile <file>` |
| `go` | `go build -o solution .` in a temp module |
| `javascript` | `node --check <file>` |
| `bash` | `bash -n <file>` |

## Run step

The run methods append the `SelfCheck` code to the same temporary file
(or a sibling file for Go) and execute the language interpreter in a
sandboxed subprocess. The sandbox is the existing `internal/sandbox`
package: on Linux it uses Landlock, on other platforms it degrades to a
no-op with a warning while still enforcing timeouts.

## Safety

- A fresh temp directory is created for every compile and every run.
- `os.RemoveAll` cleans up after each step.
- The sandbox policy denies network access and restricts writes to the
  temp directory.
- `context.WithTimeout` bounds both compile and run.

## Related files

- `scorer.go` — `CompileAndRun` type and `Score` entry point
- `types.go` — `EvalCase.Scorer` config field
- `internal/sandbox/` — OS-level isolation for subprocesses
