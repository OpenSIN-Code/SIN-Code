# ADR-008: Go 1.25 Deferral — govulncheck Stays Warn-Only Until Stable

| Field       | Value                                    |
|-------------|------------------------------------------|
| ADR         | ADR-008                                  |
| Status      | Superseded (Go 1.25.11 released, st-gvc4 closed) |
| Date        | 2026-06-08                               |
| Deciders    | jeremy                                   |
| Supersedes  | (none)                                   |
| Related     | docs/issues/st-gvc4-govulncheck-blocking.md |

## Context

> **UPDATE (2026-06-11):** Go 1.25.11 is now stable. `govulncheck` has been switched to blocking in `release.yml`. This ADR is superseded. See `docs/issues/st-gvc4-govulncheck-blocking.md` (closed).

## Context (historical — June 2026)

Go 1.25 was in the release pipeline (expected stable ~Aug 2026). Among other things, it brings improvements to `govulncheck` (Go's official vulnerability scanner) that reduce false positives — specifically, better handling of the `GOEXPERIMENT=vmmemorylimit` flag and improved symbol resolution for the `golang.org/x/*` module family.

At that time we were on **Go 1.24.x**, which had known `govulncheck` false positives on:
- `golang.org/x/sys` (false positive: GO-2024-XXXX)
- `golang.org/x/term` (false positive: GO-2023-XXXX)
- `golang.org/x/text` (false positive: GO-2024-XXXX)

These false positives **block CI** if `govulncheck` is configured as a hard gate. The choices are:

1. **Block CI on findings** → blocked by false positives, no real security gain
2. **Warn-only** → CI passes, but findings go unnoticed
3. **Wait for Go 1.25** → defer the decision ~8 weeks, accept temporary risk

## Decision

We adopted **option 2: warn-only** for `govulncheck` until Go 1.25 stable was released. On 2026-06-11, Go 1.25.11 was confirmed stable and `govulncheck` was switched to **blocking** in `release.yml`. No new ADR was needed — the trigger condition was met and the change was purely mechanical.

### Current CI Configuration (release.yml)

```yaml
- name: Run govulncheck (warn-only until Go 1.25)
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./... || echo "::warning::govulncheck findings (non-blocking)"
```

### Future CI Configuration (after Go 1.25)

```yaml
- name: Run govulncheck
  run: |
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck ./...  # blocking; fails CI on findings
```

### Manual Security Audit (compensating control)

While `govulncheck` is warn-only, we run a **manual security audit** before each release:
1. `go list -m -u all` — list outdated dependencies
2. `govulncheck ./...` — review findings manually
3. For each finding, decide: (a) upgrade, (b) ignore with documented reason, or (c) accept risk
4. Document the decision in CHANGELOG.md under "Security" section

This manual process is acceptable at our current release cadence (monthly).

## Consequences

### Positive

1. **No false-positive CI failures** — releases not blocked on tooling bugs
2. **Manual control** — security team can review findings contextually
3. **Forced upgrade date** — Go 1.25 release triggers cleanup work (tracked in [st-gvc4](../issues/st-gvc4-govulncheck-blocking.md))

### Negative

1. **Vulnerabilities can ship unnoticed** — until Go 1.25, a real CVE in our deps would not block CI
   - Mitigation: manual audit before each release; subscribe to `golang-announce` mailing list
2. **Inconsistent signal** — CI says "no errors" even when there are findings
   - Mitigation: `::warning::` annotations appear in GitHub Actions UI
3. **Tech debt** — easy to forget to flip the switch when Go 1.25 ships
   - Mitigation: issue [st-gvc4](../issues/st-gvc4-govulncheck-blocking.md) tracks this

## Trigger to Revisit

All trigger conditions were met on 2026-06-11:
- Go 1.25.11 stable is released (Go 1.26.4 is also available)
- `go.mod` is updated to `go 1.25.11`
- CI matrix is updated to test on `1.25.11`
- `release.yml` `govulncheck` step is changed to blocking (no `|| true`, no stdlib carve-out)

This ADR is now marked **Superseded**. No follow-up ADR was needed — the decision was pre-made and the execution was mechanical.

## Alternatives Considered

### Pin to a `govulncheck` version that has fewer false positives
**Considered, rejected** — `govulncheck` updates frequently, and the version we pin might have different bugs. Better to wait for Go 1.25.

### Use a different vulnerability scanner (e.g. Snyk, Trivy)
**Considered, deferred** — Third-party tools have their own quirks. `govulncheck` is the official tool and integrates best with Go. We can revisit when we have a security engineer on the team.

### Run `govulncheck` only on release tags, not on every PR
**Considered, rejected** — We want to catch vulnerabilities as early as possible. Running on every PR is the right default.

## References

- Issue: [st-gvc4](../issues/st-gvc4-govulncheck-blocking.md)
- Go release schedule: https://go.dev/doc/devel/release
- govulncheck docs: https://go.dev/security/vuln/
- Current CI: `.github/workflows/release.yml`
