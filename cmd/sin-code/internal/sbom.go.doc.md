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
