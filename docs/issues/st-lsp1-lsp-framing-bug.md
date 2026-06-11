# Issue: st-lsp1 — LSP framing bug (initialize fails on gopls v0.20+)

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-lsp1                                                     |
| Title       | Fix LSP framing bug — `Client.Call` can't read gopls v0.20+ responses |
| Status      | open                                                        |
| Priority    | P1 (blocks LSP users on modern gopls)                       |
| Created     | 2026-06-11T12:00:00Z                                        |
| Reporter    | jeremy (pro-coder audit)                                    |
| Doc         | [docs/lsp-known-issues.md#1](../lsp-known-issues.md)        |
| Component   | internal/lsp/client.go                                      |
| Effort      | 1-2 hours (30-50 LOC rewrite of `Client.Call`)              |
| Blocks      | all `sin-code lsp {symbols,hover,definition,references,format}` users on gopls ≥ 0.20 |

## Summary

`Client.Call` in `internal/lsp/client.go` reads LSP stdout with `bufio.ReadString('\n')` in a loop, only matching `Content-Length:` lines. gopls v0.20+ emits JSON-RPC notifications (`window/logMessage`, `$/progress`, `client/registerCapability`) on the same stdout stream. Those notification lines desync the header reader, the eventual `io.ReadFull` reads past the body start, and the result fails `json.Unmarshal` with `unexpected end of JSON input`.

## Symptoms

Every `sin-code lsp {symbols,hover,definition,references,format,diagnostics}` call against a `.go` file exits 1 with:
```
Error: initialize go: unexpected end of JSON input
```

## Reproduction

```bash
brew install gopls  # 0.22.0+
cd /Users/jeremy/dev/SIN-Code-Bundle
sin-code lsp symbols cmd/sin-code/main.go
# → Error: initialize go: unexpected end of JSON input
```

## Affected Versions

- gopls ≥ 0.20 (verified on 0.22.0)
- gopls ≤ 0.16 is reported to work because it does not emit notifications during `initialize`

## Workarounds

1. Pin gopls: `go install golang.org/x/tools/gopls@v0.16.0`
2. (informational only) Disable `-rpc.trace` — it's off by default in v0.22

## Suggested Fix

Rewrite `Client.Call` to read the full LSP frame (`Content-Length` + body) at once with `bufio.Scanner` (custom split) or a length-prefixed helper. Drain any non-response frames (notifications / requests from server) into a goroutine for `Call` to pick up only responses. Approx. 30-50 LOC.

## Acceptance Criteria

- [ ] `sin-code lsp symbols` works against gopls v0.22+ on a Go file
- [ ] No regression: works against gopls v0.16 too
- [ ] `lsp_live.txt` testscript extended to verify with a notification-emitting gopls version
- [ ] CI captures gopls version used for the live test

## Related

- [docs/lsp-known-issues.md](../lsp-known-issues.md) — full bug analysis
- [st-lsp2](st-lsp2-lsp-codocs-missing.md) — Missing CoDocs for lsp package
- [st-lsp3](st-lsp3-lsp-testdata-not-in-ci.md) — `lsp_live.txt` not wired into CI
