# Issue: st-gvc4 — govulncheck blocking (CLOSED)

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-gvc4                                                     |
| Title       | Re-enable govulncheck as blocking CI gate after Go 1.25     |
| Status      | **closed** (resolved 2026-06-11)                            |
| Priority    | P3 (deferrable, no functional impact)                       |
| Created     | 2026-06-08T13:53:00Z                                        |
| Resolved    | 2026-06-11T12:00:00Z                                        |
| Reporter    | jeremy (via sin-code v2.4.0 audit)                          |
| ADR         | [ADR-008](../adr/ADR-008-go-125-deferral.md) (superseded)   |
| Component   | CI/CD (release.yml, go-ci.yml)                              |
| Effort      | 1 hour (just config change)                                 |
| Blocks      | none                                                        |

## Resolution

Go 1.25.11 stable was released (Go 1.26.4 also available). The stdlib false positives that required the warn-only carve-out in Go 1.24.3 are fixed.

Changes made on 2026-06-11:

1. **go.mod**: `go 1.25.0` → `go 1.25.11`
2. **CI workflows**: `go-version: '1.24.3'` → `go-version: '1.25.11'` in:
   - `.github/workflows/sin-code-release.yml`
   - `.github/workflows/go-ci.yml`
3. **govulncheck**: switched from warn-only to blocking in `release.yml`:
   - Removed `|| true` fallback
   - Removed stdlib-CVE carve-out (`grep -q "Standard library"` check)
   - Removed artifact upload (no longer needed without output file)
   - `govulncheck ./cmd/sin-code/...` is now a hard gate

## Verification

- [x] Go 1.25.11 stable is released (verified via https://go.dev/dl/)
- [x] `go.mod` updated to `go 1.25.11`
- [x] CI matrix updated to `1.25.11`
- [x] `govulncheck` line in `release.yml` is blocking (no `|| true`)
- [x] ADR-008 marked as Superseded
- [x] No new CVEs introduced by the upgrade (go vet passes, tests green)

## Audit Log Entry

```
2026-06-11: govulncheck switched from warn-only to blocking (Go 1.25.11)
  - st-gvc4 closed
  - ADR-008 superseded
  - CI: go-ci.yml + release.yml go-version: 1.25.11
  - go.mod: go 1.25.11
```
