# `security.doc.md` — Security Scan Subcommand

Runs a fast, targeted security analysis tailored to the project type detected at the given path.

## What it does

- **Auto-detects project type** by looking for `go.mod`, `package.json`, `requirements.txt`, `pyproject.toml`, `setup.py`, or `Pipfile`.
- **Runs available security tools** for that type:
  - **Go:** `govulncheck`, `gosec`, `go vet`, `grype` (Go-native SCA, issue #41)
  - **Python:** `bandit`, `safety`
  - **Node.js:** `npm audit`
  - **Generic:** `secrets grep` (high-entropy strings), file-permission checks
- **Produces a concise summary** with per-tool status, issue count, and total duration.

## `security scan secrets` — vendored secrets scanner

`security scan secrets` runs the vendored `SIN-Code-Secrets-Scanner` tool
(22+ detection rules, entropy filtering, severity classification). It locates
the `sin-secrets` binary in the following order and falls back to compiling it
from the vendored module into the user cache:

1. `$SIN_SECRETS_BIN`
2. A binary named `sin-secrets` on `PATH`
3. The vendored `SIN-Code-Secrets-Scanner` module (built on demand with `CGO_ENABLED=0`)

Findings are masked in the output and a machine-readable JSON format is
available for CI pipelines.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `SecurityCmd` into the root cobra command
- `cmd/sin-code/internal/security.go` — parent `security` command
- `cmd/sin-code/internal/security_secrets.go` — `security scan secrets` implementation
- `cmd/sin-code/internal/security_test.go` — unit tests for detection and scan logic
- `cmd/sin-code/internal/security_secrets_test.go` — tests for the secrets scanner bridge
- `cmd/sin-code/internal/common.go` — may share `runWithTimeout` helper

## Important config values & limits

- `--timeout` default: **300 seconds** per tool
- `--format` default: `text` (also supports `json`)
- `--type` default: `auto` (can be forced to `go`, `python`, `node`, `generic`)
- `--strict` flag: exits with error code if any issues are found

## Usage examples

```bash
# Auto-detect and scan current directory
sin-code security

# Force Go project scan with JSON output and strict mode
sin-code security ./backend --type go --format json --strict

# Only run specific tools (whitelist)
sin-code security . --tools govulncheck,gosec

# Run the vendored secrets scanner on the current workspace
sin-code security scan secrets

# Secrets scan with severity filter and JSON output
sin-code security scan secrets ./src --severity high --format json --strict

# Do not build the vendored scanner if the binary is missing
sin-code security scan secrets --no-build
```

## Known caveats / footguns

- **Tool availability:** If a tool is not installed, it is marked `not_found` and skipped. No automatic installation is attempted.
- **File-permission scan root:** If the scan root itself is unreadable (e.g., missing directory), `runFilePermissions` returns an error instead of silently reporting zero files. Unreadable individual entries inside a readable root are still skipped.
- **File-permission scan testability:** The unexported `dirEntryInfo` hook lets tests simulate `fs.DirEntry.Info()` failures deterministically.
- **Issue counting is heuristic:** For some tools (e.g., `go vet`), we count lines in output; this may not perfectly match the tool's native issue count.
- **Secrets grep is basic:** The generic `security` command uses simple regexes. For deeper detection, use `security scan secrets` which runs the vendored secrets scanner.
- **Secrets scanner build:** The first run may compile the vendored scanner into the user cache (`$UserCacheDir/sin-code/sin-secrets`). Set `--no-build` to fail fast instead.
- **Exit codes:** Without `--strict`, the command returns `0` even if issues are found. CI pipelines should use `--strict` to fail on issues.
- **Timeout is per-tool:** A slow `npm audit` on a large monorepo can exceed the 300s default. Increase with `--timeout`.
## MCP exposure (v3.11.0, issue #36)

`sin_security_scan` is exposed via `sin-code serve` since v3.11.0. Same arguments
as the CLI flags (`--type`, `--tools`, `--format`, `--timeout`, `--strict`);
output is JSON by default (CLI default is `text`). Race-clean, bounded by
`--timeout` (max 3600s at the MCP layer; per-tool timeout is still enforced by
`runWithTimeout` in security.go). The `strict` flag is accepted by the MCP
handler but does NOT propagate as an MCP error — the caller inspects the JSON
`Summary.Issues` field instead.

Permission default: `allow` (read-only — never mutates the scanned tree).

