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
- **SARIF 2.1.0 output** via `--format sarif` for the `security`, `security scan secrets`, `security scan sast`, and `security scan all` commands, so findings can be consumed by GitHub Advanced Security, Azure DevOps, and other SARIF-compatible gateways.

## `security scan all` — aggregate secrets + SAST

`security scan all` runs the vendored secrets scanner and the vendored SAST
scanner against the given path and aggregates their findings into a single
report. It supports the same output formats as the individual scanners:

- `text` (default) — human-readable summary
- `json` — machine-readable aggregated report
- `sarif` — SARIF 2.1.0 JSON for CI/security gateways

Use `--strict` to exit with an error when any issues are found, and `--no-build`
to skip scanners that are not already present.

## `security scan secrets` — vendored secrets scanner

`security scan secrets` runs the vendored `SIN-Code-Secrets-Scanner` tool
(22+ detection rules, entropy filtering, severity classification). It locates
the `sin-secrets` binary in the following order and falls back to compiling it
from the vendored module into the user cache:

1. `$SIN_SECRETS_BIN`
2. A binary named `sin-secrets` on `PATH`
3. The vendored `SIN-Code-Secrets-Scanner` module (built on demand with `CGO_ENABLED=0`)

Findings are masked in the output and a machine-readable JSON format is
available for CI pipelines. `--format sarif` emits SARIF 2.1.0 JSON with the
scanner set to `sin-secrets`.

## `security scan sca` — vendored SCA scanner

`security scan sca` runs the vendored `SIN-Code-SCA-Tool-Go` software composition
analysis scanner. It parses dependency lock files (`go.mod`, `package-lock.json`,
`requirements.txt`, `pom.xml`) and queries vulnerabilities via OSV.dev. It locates
the `sin-sca-go` binary in the following order and falls back to compiling it
from the vendored module into the user cache:

1. `$SIN_SCA_BIN`
2. A binary named `sin-sca-go` on `PATH`
3. The vendored `SIN-Code-SCA-Tool-Go` module (built on demand with `CGO_ENABLED=0`)

JSON output is available for CI pipelines, and `--severity` filters the reported
vulnerabilities by minimum severity.

## `security scan sast` — vendored SAST scanner

`security scan sast` runs the vendored `SIN-Code-SAST-Tool` static analysis
scanner. It finds common security anti-patterns in Go source code and classifies
them by severity. The scanner binary is located in the following order and falls
back to compiling it from the vendored module into the user cache:

1. `$SIN_SAST_BIN`
2. A binary named `sin-sast-go` on `PATH`
3. The vendored `SIN-Code-SAST-Tool` module (built on demand with `CGO_ENABLED=0`)

Supported output formats:

- `text` (default)
- `json`
- `sarif` — SARIF 2.1.0 JSON

## `security scan sbom` — vendored SBOM generator

`security scan sbom` generates a Software Bill of Materials (SBOM) for the
project at the given path. It auto-detects the manifest file (`go.mod`,
`package.json`, `requirements.txt`), collects dependencies, and runs the
vendored `SIN-Code-SBOM-Generator-Go` to produce SPDX 2.3 or CycloneDX 1.5 JSON.

The generator binary is located in the following order and falls back to
compiling it from the vendored module into the user cache:

1. `$SIN_SBOM_BIN`
2. A binary named `sin-sbom-go` on `PATH`
3. The vendored `SIN-Code-SBOM-Generator-Go` module (built on demand with `CGO_ENABLED=0`)

Supported output formats:

- `spdx-json` (default)
- `cyclonedx-json`

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `SecurityCmd` into the root cobra command
- `cmd/sin-code/internal/security.go` — parent `security` command
- `cmd/sin-code/internal/security_secrets.go` — `security scan` subcommand tree, `security scan all`, and `security scan secrets` implementation
- `cmd/sin-code/internal/security_scan_sast.go` — `security scan sast` implementation
- `cmd/sin-code/internal/security_sarif.go` — SARIF 2.1.0 converter and unified `SecurityFinding` model
- `cmd/sin-code/internal/security_scan_sca.go` — `security scan sca` implementation
- `cmd/sin-code/internal/security_scan_sbom.go` — `security scan sbom` implementation
- `cmd/sin-code/internal/security_test.go` — unit tests for detection and scan logic
- `cmd/sin-code/internal/security_secrets_test.go` — tests for the secrets scanner bridge
- `cmd/sin-code/internal/security_scan_sast_test.go` — tests for the SAST scanner bridge
- `cmd/sin-code/internal/security_scan_sca_test.go` — tests for the SCA scanner bridge
- `cmd/sin-code/internal/security_scan_sbom_test.go` — tests for the SBOM generator bridge
- `cmd/sin-code/internal/common.go` — may share `runWithTimeout` helper

## Important config values & limits

- `--timeout` default: **300 seconds** per tool
- `--format` default: `text` (also supports `json`, `sarif`)
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

# Secrets scan with SARIF output for GitHub Advanced Security
sin-code security scan secrets ./src --format sarif --strict > results.sarif

# Do not build the vendored scanner if the binary is missing
sin-code security scan secrets --no-build

# Run the vendored SAST scanner on the current workspace
sin-code security scan sast

# SAST scan with SARIF output
sin-code security scan sast ./src --format sarif > sast.sarif

# Run aggregate secrets + SAST scan and emit SARIF
sin-code security scan all ./src --format sarif --strict > all.sarif

# Run the vendored SCA scanner on the current workspace
sin-code security scan sca

# SCA scan with severity filter and JSON output
sin-code security scan sca ./src --severity high --format json --strict

# Do not build the vendored SCA scanner if the binary is missing
sin-code security scan sca --no-build

# Generate an SPDX 2.3 JSON SBOM for the current project
sin-code security scan sbom

# Generate a CycloneDX 1.5 JSON SBOM for a specific path
sin-code security scan sbom ./backend --format cyclonedx-json

# Custom SBOM document name
sin-code security scan sbom ./ --name my-service --format spdx-json

# Do not build the vendored SBOM generator if the binary is missing
sin-code security scan sbom --no-build
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

## Audit integration (v3.24.0)

The `RunSecurityAudit` / `RunSecurityAuditWithTimeout` helpers are used by the
`audit` and `ceo-audit` commands so that security findings are surfaced
without requiring a separate `sin-code security` invocation.

- `sin-code audit security [<path>] [--format json] [--strict] [--timeout 30]`
  runs the same lightweight detection used by `security`, but capped at one
  fast tool per project type (`go vet`, `bandit`, `npm audit`, or `secrets grep`)
  to keep runtime under the audit overhead budget.
- `sin-code ceo-audit` populates the existing `security-scan` gate (gate 3) with
  `RunSecurityAuditWithTimeout`. In `--strict` mode, any findings cause the
  audit to exit with an error; by default they are reported as `warn` and do not
  affect the A+ score.

