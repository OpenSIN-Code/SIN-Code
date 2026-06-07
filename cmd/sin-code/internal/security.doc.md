# `security.doc.md` — Security Scan Subcommand

Runs a fast, targeted security analysis tailored to the project type detected at the given path.

## What it does

- **Auto-detects project type** by looking for `go.mod`, `package.json`, `requirements.txt`, `pyproject.toml`, `setup.py`, or `Pipfile`.
- **Runs available security tools** for that type:
  - **Go:** `govulncheck`, `gosec`, `go vet`
  - **Python:** `bandit`, `safety`
  - **Node.js:** `npm audit`
  - **Generic:** `secrets grep` (high-entropy strings), file-permission checks
- **Produces a concise summary** with per-tool status, issue count, and total duration.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `SecurityCmd` into the root cobra command
- `cmd/sin-code/internal/security_test.go` — unit tests for detection and scan logic
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
```

## Known caveats / footguns

- **Tool availability:** If a tool is not installed, it is marked `not_found` and skipped. No automatic installation is attempted.
- **Issue counting is heuristic:** For some tools (e.g., `go vet`), we count lines in output; this may not perfectly match the tool's native issue count.
- **Secrets grep is basic:** Uses simple regexes. It is NOT a replacement for `truffleHog` or `git-secrets`.
- **Exit codes:** Without `--strict`, the command returns `0` even if issues are found. CI pipelines should use `--strict` to fail on issues.
- **Timeout is per-tool:** A slow `npm audit` on a large monorepo can exceed the 300s default. Increase with `--timeout`.
