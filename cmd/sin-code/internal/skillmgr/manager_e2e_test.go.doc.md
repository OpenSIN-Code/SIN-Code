What it does: E2E integration test for the `websearch` skill installation in SIN-Code. Clones `web_search_bundle` from GitHub, builds the `sin-websearch` binary, and verifies it runs `--help`.

Dependencies: Network access to GitHub and a working Go toolchain. Gated behind the `e2e` build tag.

Usage:
```bash
go test -tags=e2e ./cmd/sin-code/internal/skillmgr/...
```

Caveats:
- Slow (~1-5 minutes) because it clones and builds from source.
- Can fail due to network issues or GitHub availability.
- Not run in normal unit-test CI; requires a separate E2E workflow.
