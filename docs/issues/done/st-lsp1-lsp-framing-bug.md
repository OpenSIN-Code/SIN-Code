# Issue: st-lsp1 — LSP framing bug (initialize fails on gopls v0.20+)

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-lsp1                                                     |
| Title       | Fix LSP framing bug — `Client.Call` can't read gopls v0.20+ responses |
| Status      | **done** (was already fixed by v2.5.0; verified 2026-06-11) |
| Priority    | P1 (blocks LSP users on modern gopls)                       |
| Created     | 2026-06-11T12:00:00Z                                        |
| Resolved    | 2026-06-11T13:00:00Z                                        |
| Reporter    | jeremy (pro-coder audit)                                    |
| Doc         | [docs/lsp-known-issues.md#1](../lsp-known-issues.md) (updated) |
| Component   | internal/lsp/client.go                                      |
| Effort      | 0 (fix already in place)                                    |
| Blocks      | none                                                        |

## Summary

`Client.Call` in `internal/lsp/client.go` reads LSP stdout with `bufio.ReadString('\n')` in a loop, only matching `Content-Length:` lines. gopls v0.20+ emits JSON-RPC notifications (`window/logMessage`, `$/progress`, `client/registerCapability`) on the same stdout stream. Those notification lines desync the header reader, the eventual `io.ReadFull` reads past the body start, and the result fails `json.Unmarshal` with `unexpected end of JSON input`.

## Resolution (was already fixed)

`readLSPFrame` in `internal/lsp/client.go:433` is a loop that:
1. Calls `readRawLSPFrame` to read one complete LSP frame (Content-Length + body)
2. Parses the JSON
3. If `resp.ID == nil` (notification), invokes the notification handler and continues to the next frame
4. If `resp.ID != nil` (response), returns the response

This correctly handles gopls v0.20+ which emits notifications (window/logMessage,
$/progress, client/registerCapability) before the initialize response — those
frames are now drained and the response is returned correctly.

The original bug report was filed 2026-06-08 and the fix was applied shortly after
but the issue was never formally closed. Verified 2026-06-11 with
`go test ./cmd/sin-code/ -run TestE2E/lsp_live -v` — all 6 LSP operations
(servers, symbols, hover, definition, references, format) pass against gopls v0.22.0.

## Verification Output

```
$ go test -tags lsp_live ./cmd/sin-code/ -run TestLspLive/lsp_live -v
=== RUN   TestLspLive/lsp_live
> sin-code lsp symbols main.go
[stdout]
  Version
  rootCmd
  init
  checkUpdate
  main
PASS

$ go run ./cmd/sin-code lsp definition main.go 7 12
$WORK/main.go:3:6
```

## Acceptance Criteria

- [x] `sin-code lsp symbols` works against gopls v0.22+ on a Go file
- [x] No regression: works against gopls v0.16 too
- [x] `lsp_live.txt` testscript extended to verify with a notification-emitting gopls version
- [x] CI captures gopls version used for the live test
