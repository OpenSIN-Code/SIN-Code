# Release Process

## Overview

SIN-Code releases are triggered by pushing a git tag. Goreleaser builds, signs, and publishes automatically.

## Pre-release checklist

1. **CEO Audit**: `sin-code ceo-audit .` — must score 100/A+, 48/48 gates
2. **Test suite**: `go test -race -count=1 ./cmd/sin-code/... -timeout 600s` — all green
3. **Build**: `CGO_ENABLED=0 go build ./cmd/sin-code/` — clean
4. **Vet**: `go vet ./...` — clean
5. **CHANGELOG.md**: Add `## [vX.Y.Z] - YYYY-MM-DD` section with all changes
6. **AGENTS.md**: Update "Last verified against main" line
7. **README.md**: Update version badge if needed

## Release steps

1. `git tag vX.Y.Z && git push origin vX.Y.Z`
2. GitHub Actions `release.yml` triggers:
   - Goreleaser builds linux/darwin/windows × amd64/arm64
   - CGO_ENABLED=0 (M2 static binary)
   - SBOM generated via syft (SPDX JSON)
   - Cosign keyless signing via OIDC
   - Homebrew formula pushed to `OpenSIN-Code/homebrew-sin`
   - Build provenance attestation
3. Verify release:
   - `gh release view vX.Y.Z --repo OpenSIN-Code/SIN-Code`
   - `brew install sin-code && sin-code --version`
   - `cosign verify-blob --certificate-identity-regexp '.*' --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' checksums.txt`

## Python releases (separate)

Python companion (`src/sin_code_bundle/`) uses separate `python-v*` tags and `release-python.yml`.

## Version naming

- Major (vX.0.0): breaking changes, MCP tool renames
- Minor (vX.Y.0): new features, new subcommands
- Patch (vX.Y.Z): bug fixes, doc updates
