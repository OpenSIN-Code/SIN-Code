# SIN-Code SBOM Generator (Go-Native)

Software Bill of Materials (SBOM) generator that creates SPDX 2.3 and CycloneDX 1.5 JSON documents from dependency scan results.

## Features

- **Dual format support**: SPDX 2.3 + CycloneDX 1.5 JSON
- **Vulnerability annotation**: Maps vulnerability scan results to SBOM packages
- **Dependency graph**: Full dependency relationships in both formats
- **License tracking**: Extracts and deduplicates license information
- **Source type detection**: Auto-detects npm, PyPI, Go, Maven, etc.
- **Zero dependencies**: Single static binary, no Python runtime needed

## Installation

```bash
go build -o sin-sbom-go ./cmd/sbom
```

## Usage

```bash
# Generate from SCA results JSON
./sin-sbom-go -sca-results sca_results.json -output-spdx sbom.spdx.json -output-cyclonedx sbom.cyclonedx.json

# Generate from raw dependencies JSON
./sin-sbom-go -deps deps.json -output-spdx sbom.spdx.json -output-cyclonedx sbom.cyclonedx.json

# Generate with custom document name
./sin-sbom-go -deps deps.json -name "my-project-sbom" -output-spdx out.spdx.json

# Generate summary markdown
./sin-sbom-go -deps deps.json -output-summary summary.md
```

## Supported Input Formats

### SCA Results JSON
```json
{
  "packages": [
    {"name": "lodash", "version": "4.17.21", "license": "MIT", "type": "library"}
  ],
  "vulnerabilities": [
    {"package": "lodash", "severity": "high", "cve": "CVE-2021-23337"}
  ],
  "files_scanned": ["package.json"]
}
```

### Dependencies JSON
```json
[
  {"name": "requests", "version": "2.31.0", "license": "Apache-2.0", "purl": "pkg:pypi/requests@2.31.0"}
]
```

## SPDX 2.3 Output
- Full compliance with SPDX 2.3 specification
- Document creation info with tool and organization
- Packages with PURL, CPE, checksums, licenses
- Dependency relationships (DESCRIBES, DEPENDS_ON)
- Extracted licensing information for non-SPDX licenses

## CycloneDX 1.5 Output
- Full compliance with CycloneDX 1.5 specification
- Metadata with tool info and authors
- Components with hashes, licenses, properties
- Dependency graph with `dependsOn`
- Security vulnerability properties (`sin:security:*`)

## Testing

```bash
go test -v ./...
```

## Architecture

- `pkg/models` - Shared data models (SBOM, SBOMPackage, SBOMMetadata)
- `internal/spdx` - SPDX 2.3 JSON generator
- `internal/cyclonedx` - CycloneDX 1.5 JSON generator
- `internal/generator` - Main orchestration (SCA results parsing, vulnerability annotation, source type detection)
- `cmd/sbom` - CLI entry point

## License

MIT - OpenSIN-Code Project
