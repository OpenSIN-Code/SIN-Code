# SIN-Code SBOM Generator — Phase 9

📊 **SBOM Generator** — Generate SPDX 2.3 and CycloneDX 1.5 Software Bill of Materials from security scan results.

## Overview

Generate standardized, machine-readable SBOMs from SCA (Software Composition Analysis) scan results. Supports both major SBOM formats:

- **SPDX 2.3** (JSON) — Linux Foundation standard
- **CycloneDX 1.5** (JSON) — OWASP standard

## Features

- 🔄 **Dual format support** — Generate both SPDX and CycloneDX in one run
- 📦 **Package extraction** — Parses SCA scan results (npm, pypi, go, maven, etc.)
- 🔗 **PURL & CPE support** — Package URLs and Common Platform Enumeration identifiers
- 🛡️ **Vulnerability annotations** — Maps vulnerability data to SBOM components
- 📋 **License tracking** — SPDX license identifiers and custom license declarations
- 🔗 **Dependency graph** — Full dependency relationships in both formats
- 📝 **Human-readable summary** — Markdown summary for quick review
- 🚀 **CLI with rich output** — Beautiful tables and progress indicators

## Quick Start

```bash
# Install
pip install -e ".[dev]"

# Generate from SCA results
sin-sbom generate sca-results.json --format both --output ./sboms

# Generate from raw dependency list
sin-sbom from-deps --name my-app --packages '[{"name":"lodash","version":"4.17.21","license":"MIT"}]'

# Summary of existing SBOM
sin-sbom summary ./sboms/my-app.spdx.json

# List tools
sin-sbom --help
```

## Architecture

| Component | Purpose |
|-----------|---------|
| `src/sbom_generator/models.py` | Data models (SBOM, SBOMPackage, SBOMMetadata) |
| `src/sbom_generator/generator.py` | Main orchestrator — aggregates data from SCA results |
| `src/sbom_generator/spdx_generator.py` | SPDX 2.3 JSON format generator |
| `src/sbom_generator/cyclonedx_generator.py` | CycloneDX 1.5 JSON format generator |
| `src/sbom_generator/cli.py` | Click-based CLI with rich output |

## Supported Package Managers

| Package Manager | Source Files | SBOM Format |
|-----------------|-------------|-------------|
| **npm** | package.json, package-lock.json, yarn.lock | SPDX + CycloneDX |
| **PyPI** | requirements.txt, poetry.lock, Pipfile.lock | SPDX + CycloneDX |
| **Go** | go.mod, go.sum | SPDX + CycloneDX |
| **Maven** | pom.xml | SPDX + CycloneDX |
| **Gradle** | build.gradle | SPDX + CycloneDX |
| **Cargo** | Cargo.toml, Cargo.lock | SPDX + CycloneDX |
| **RubyGems** | Gemfile, Gemfile.lock | SPDX + CycloneDX |
| **Composer** | composer.json, composer.lock | SPDX + CycloneDX |

## Integration with SIN-Code Security Bundle

```bash
# Via bundle CLI (automatically generates SBOM after SCA scan)
sin-security scan ./my-project --sbom

# Generate SBOM from SCA results
sin-sbom generate sca-results.json --format both

# Output files
# - sbom.spdx.json
# - sbom.cyclonedx.json
# - sbom-summary.md
```

## Output Example

### SPDX 2.3 JSON
```json
{
  "spdxVersion": "SPDX-2.3",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "my-app-sbom",
  "packages": [
    {
      "SPDXID": "SPDXRef-Package-0",
      "name": "lodash",
      "versionInfo": "4.17.21",
      "licenseConcluded": "MIT",
      "downloadLocation": "NOASSERTION"
    }
  ],
  "relationships": [
    {
      "spdxElementId": "SPDXRef-DOCUMENT",
      "relatedSpdxElement": "SPDXRef-Package-0",
      "relationshipType": "DESCRIBES"
    }
  ]
}
```

### CycloneDX 1.5 JSON
```json
{
  "bomFormat": "CycloneDX",
  "specVersion": "1.5",
  "components": [
    {
      "type": "library",
      "name": "lodash",
      "version": "4.17.21",
      "licenses": [{"license": {"id": "MIT"}}]
    }
  ]
}
```

## License

MIT — See [LICENSE](../LICENSE)
