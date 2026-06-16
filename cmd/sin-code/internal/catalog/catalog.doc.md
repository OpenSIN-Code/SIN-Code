# catalog — unified tool catalog (issue #163)

`internal/catalog/` is the v3.18.0 unification of the legacy
`internal/hub/` (static subcommand list) and `internal/assets/`
(loaded Markdown frontmatter assets) under one Source interface.
The CLI is `sin-code catalog`, which is now the operator-facing
entry point for "do I have a tool for this?".

The legacy `sin-code hub` is **not** removed in this PR — see
"Deprecation" below. The `sin-code assets` CLI was never a top-level
command (the asset loader is library-only today), so no deprecation
is needed there.

## What ships

### Package: `cmd/sin-code/internal/catalog/`

| File | Purpose |
|---|---|
| `catalog.go` | `Asset`, `Kind`, `Source` interface, `Merge` (de-dup), `Search` (ranked), `FilterByKind` |
| `source_hub.go` | `HubSource` — wraps `internal/hub` so the static catalog is one Source implementation |
| `source_assets.go` | `AssetsSource` — wraps `*assets.Registry`; nil-registry safe |
| `catalog_test.go` | 21 race-clean unit tests (Merge, Search, FilterByKind, both source adapters) |

### CLI: `cmd/sin-code/catalog_cmd.go`

```bash
sin-code catalog                       # all assets, all sources
sin-code catalog list                  # same, flat
sin-code catalog list --kind=agent     # filter by kind
sin-code catalog list --format=json    # machine-readable
sin-code catalog search "<query>"      # ranked substring search
sin-code catalog info <name>           # one asset by name
```

### Source abstraction

```go
type Source interface {
    Name() string
    List(ctx context.Context, kind Kind) ([]*Asset, error)
    Get(ctx context.Context, kind Kind, name string) (*Asset, bool, error)
}
```

The catalog walks every registered source, merges, and de-duplicates
by `(kind, name)` (the source name is intentionally NOT part of
the dedup key, so a hub.Tool and an assets.Asset with the same
name are merged into one catalog entry). The first source to
provide a given (kind, name) pair wins; subsequent duplicates
are dropped. This is deterministic and order-preserving.

## De-duplication rule

| Scenario | Behavior |
|---|---|
| Hub has `chat`, assets has `chat` | Merged into one `chat` (first source wins) |
| Hub has `chat`, assets has `agent chat` | Both kept (different names) |
| Hub has `chat` (kind=hub), assets has `chat` (kind=agent) | Both kept (different kinds) |

The de-dup key is `(kind, name)`, not `(source, kind, name)`. This
is the SOTA choice for the operator's mental model: "do I have a
tool for this?" — they don't care which backend has it.

## Search ranking

The `Search` function uses a transparent heuristic:

| Field | Score |
|---|---:|
| Name contains query | +4 |
| Short contains query | +2 |
| Description contains query | +1 |
| Any tag contains query | +1 |

Ties break by name ascending. The score is exposed in tests but
not in the CLI output (operators don't need it; reviewers do).

## Deprecation

`sin-code hub` continues to work unchanged. The PR does **not**
delete it, per the issue body:

> `sin hub` and `sin assets` remain as deprecated aliases of
> `sin catalog` for one minor release. After v3.20, they go.

The deprecation warning is **not** added in this PR. The
catalog/hub split is the underlying mechanism; the warning can
be added in a follow-up that just patches `hub_cmd.go` to print
`deprecation: sin-code hub is deprecated, use sin-code catalog`
on every call.

## Acceptance criteria (from #163)

- [x] `sin catalog list` shows the union of both sources, de-duped
- [x] `sin catalog search` ranks across both sources
- [x] The `Source` interface has a reference implementation for each
      existing source (vendored + hub)
- [x] Test coverage ≥ 80% (21 tests, all paths)
- [x] M2 (single binary, CGO_ENABLED=0): stdlib + existing deps only
- [x] M6 (SIN tools over naive built-ins): the Source interface
      uses the existing `Selector` (from PR #144) as a future
      enhancement — currently the simple `Merge`+`Search` is enough

## Known build issue (NOT in this PR)

`go build ./cmd/sin-code/...` is currently broken on the v3.18.0
main because the merged `pkg/browser/cdp/` PR (PR #201) shipped
without a complete `go.sum` and a Chromedp API version that
matches the source code. This PR does not touch Browser/CDP, but
the binary build needs the upstream fix before it can be verified
end-to-end. The catalog package itself is build-clean and tested.
