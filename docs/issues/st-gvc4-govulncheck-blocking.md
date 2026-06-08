# Issue: st-gvc4 — govulncheck blocking depends on Go 1.25

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-gvc4                                                     |
| Title       | Re-enable govulncheck as blocking CI gate after Go 1.25     |
| Status      | open (blocked on Go 1.25 stable release)                    |
| Priority    | P3 (deferrable, no functional impact)                       |
| Created     | 2026-06-08T13:53:00Z                                        |
| Reporter    | jeremy (via sin-code v2.4.0 audit)                          |
| ADR         | [ADR-008](../adr/ADR-008-go-125-deferral.md)               |
| Component   | CI/CD (release.yml)                                         |
| Effort      | 1 hour (just config change)                                 |
| Blocks      | none                                                        |

## Summary

Go 1.25 introduces `GOEXPERIMENT=vmmemorylimit`-aware `govulncheck` with reduced false positives. We're on Go 1.24.x today. Until Go 1.25 ships stable, the CI pipeline runs `govulncheck` in **warn-only** mode (exits 0 even on findings) to avoid false-positive build failures.

## Why Defer

- Go 1.25 stable is expected ~Aug 2026 (6-8 weeks from now)
- Go 1.24's `govulncheck` has known false positives for several `golang.org/x/*` modules we use (`x/sys`, `x/term`, `x/text`)
- Blocking on a false positive would block the v2.4.0 release for no real security gain

## Current State (release.yml)

```yaml
- name: Run govulncheck
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./... || echo "::warning::govulncheck findings (warn-only until Go 1.25)"
```

## Target State (after Go 1.25)

```yaml
- name: Run govulncheck
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...  # blocking
```

## Acceptance Criteria

- [ ] Go 1.25 stable is released
- [ ] `go.mod` updated to `go 1.25.0`
- [ ] CI matrix updated to test on `1.25.x`
- [ ] `govulncheck` line in `release.yml` removes the `|| echo` fallback
- [ ] Audit log entry: "govulncheck: switched from warn-only to blocking"
- [ ] No new CVEs introduced by the upgrade (run `govulncheck` manually first)

## Tracking

Watch: https://go.dev/doc/devel/release (for `go1.25` stable)
Action trigger: When `go version go1.25.0` is downloadable, create a PR with the above changes.

## Definition of Done

1. Go 1.25.0+ installed in CI runner
2. `go.mod` `go 1.25.0`
3. `release.yml` `govulncheck` step is blocking
4. CI run is green with the stricter check
5. v2.6.0 or v3.0.0 release notes mention "govulncheck: blocking CI gate"
