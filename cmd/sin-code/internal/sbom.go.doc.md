# sbom.go

## What

Generates SPDX 2.3 or CycloneDX 1.5 JSON SBOMs (Software Bill of Materials) for Go, Python, Node.js, and generic projects.

## Dependencies

- `security.go` — reuses `detectProjectType` for project type detection
- `serve.go` — uses `ServerVersion` for tool version metadata
- `main.go` — `SbomCmd` registered as `internal.SbomCmd`

## Important config values & limits

- Default format: `spdx-json`
- Go projects: runs `go list -m -json all` (requires Go toolchain in PATH)
- Python projects: simple regex-based parsing of `requirements.txt` and `pyproject.toml` (no full pip resolver)
- Node.js projects: parses `package.json` and resolves versions from `package-lock.json` if available
- Generic projects: lists top-level directories as components

## Usage examples

```bash
sin-code sbom .
sin-code sbom ./my-project --format cyclonedx-json --output sbom.json
```

## Known caveats

- Python dependency parsing is heuristic-based (no full pip resolver)
- Go fallback to `go.mod` parsing may miss indirect dependencies
- Generic SBOM only lists directories, not files
- License fields default to `NOASSERTION` for all packages
## MCP exposure (v3.11.0, issue #36)

`sin_sbom_generate` is exposed via `sin-code serve` since v3.11.0. Same arguments
as the CLI flags (`--format`, `--output`); output is inline JSON by default (omit
`output` or set to `-`). The `output` parameter is path-escape-guarded — any path
outside the scan root is rejected with an error to prevent the MCP layer from
being a write-anywhere primitive.

Permission default: `allow` (read-only by default — SBOM generation reads source
files but does not mutate them; the output sandbox is the belt-and-suspenders
defense, not the primary control).

