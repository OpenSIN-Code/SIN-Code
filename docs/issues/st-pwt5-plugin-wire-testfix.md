# Issue: st-pwt5 — Pre-existing failure: TestE2E/plugin_wire

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-pwt5                                                     |
| Title       | Fix pre-existing test failure in plugin_wire testscript     |
| Status      | open (pre-existing, not blocking v2.4.0)                    |
| Priority    | P2 (cleanup, not user-facing)                               |
| Created     | 2026-06-08T13:54:00Z                                        |
| Reporter    | jeremy (via sin-code v2.4.0 audit)                          |
| Component   | testscripts, plugins                                        |
| Effort      | 1-2 hours                                                   |
| Blocks      | none                                                        |

## Summary

`testdata/scripts/plugin_wire.txt` was added in v2.3.0 (commit `bc524c8`) to test the plugin system end-to-end. It has been failing on the `main` branch since then. The failure was masked because it was added alongside many other passing tests, and the v2.4.0 release was scoped to "LSP fix" (commit `63b33f5`) so this failure was carried over.

## Why P2 (not P0)

- Not a user-facing bug
- Not blocking the LSP fix (P0)
- Cleanup item, not feature gap

## Repro

```bash
go test ./... -count=1 -run TestE2E/plugin_wire
# Output: ... FAIL: testdata/scripts/plugin_wire.txt:LINE ...
```

## Likely Cause

The test was written for the v2.3.0 plugin manifest format but the manifest evolved before the test was merged. Specifically, `[[agents]]` with `provider`/`model` fields may have been renamed or moved, and the test fixture plugin uses the old shape.

## Acceptance Criteria

- [ ] Run `go test ./... -run TestE2E/plugin_wire -v` and identify the failing assertion
- [ ] Either: (a) fix the test fixture to match the current plugin manifest, OR (b) fix the test script to use the correct invocation
- [ ] `go test ./... -count=1` green
- [ ] Audit log entry: "TestE2E/plugin_wire: fixed"
- [ ] Add a comment at the top of `plugin_wire.txt` explaining what it tests

## Files Touched

- `testdata/scripts/plugin_wire.txt` — fix assertions or fixture
- Possibly: `testdata/fixtures/sample-plugin/plugin.toml` — update to current manifest schema

## Investigation Steps

1. Read `testdata/scripts/plugin_wire.txt` and identify which assertion fails
2. Cross-reference with `docs/plugin-system-design.md` for current manifest schema
3. If the test is testing deprecated manifest fields, migrate it
4. If the test is correct, fix the code to match

## Definition of Done

1. `TestE2E/plugin_wire` passes
2. `go test ./...` green
3. v2.5.0 release notes: "testscripts: fix pre-existing plugin_wire failure"
