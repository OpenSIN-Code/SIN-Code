# LSP Known Issues

This file documents bugs and limitations of the `sin-code lsp` command as of
the latest release. The LSP client lives in
`cmd/sin-code/internal/lsp/client.go` and is exercised by
`cmd/sin-code/testdata/scripts/lsp_live.txt`.

## 1. `initialize go: unexpected end of JSON input` on every call

**Status:** Open (regression introduced in the initial LSP commit;
  discovered via live testing on 2026-06-08 against gopls v0.22.0.)

**Symptom**

Every `sin-code lsp {symbols,hover,definition,references,format,diagnostics}`
call against a `.go` file exits with code 1 and prints:

```
Error: initialize go: unexpected end of JSON input
```

**Root cause**

`Client.Call` in `internal/lsp/client.go` (line 376) reads the LSP stdout
header with `bufio.ReadString('\n')` in a loop, only matching `Content-Length:`
lines. gopls v0.20+ emits JSON-RPC notifications (`window/logMessage`,
`$/progress`, `client/registerCapability`) on the same stdout stream before
the initialize response. Those notification lines do not match the
`Content-Length:` predicate, the loop burns through them, and the eventual
`io.ReadFull` reads past the start of the body, yielding truncated JSON
that fails `json.Unmarshal` with "unexpected end of JSON input".

**Affected versions**

- gopls ≥ 0.20 (verified on 0.22.0)
- gopls ≤ 0.16 is reported to work because it does not emit notifications
  during `initialize`.

**Reproduction**

```bash
brew install gopls
cd /Users/jeremy/dev/SIN-Code-Bundle
sin-code lsp symbols cmd/sin-code/main.go
# → Error: initialize go: unexpected end of JSON input
```

**Workarounds**

- Pin gopls: `go install golang.org/x/tools/gopls@v0.16.0`
- Or: spawn gopls with `-rpc.trace` disabled (it is by default in v0.22,
  so this is informational, not a workaround).

**Suggested fix**

Rewrite `Client.Call` to read the full LSP frame (`Content-Length` + body) at
once with `bufio.Scanner` (custom split) or a length-prefixed helper, and
drain any non-response frames (notifications / requests from server) into a
goroutine. Approx. 30-50 LOC.

## 2. UX: `--file` flag collides with positional arg

**Status:** Minor (not blocking)

`sin-code lsp symbols --file main.go` fails with
`accepts between 1 and 2 arg(s), received 0`. When `--file` is passed the
cobra command still requires a positional `<file>` argument. Either drop
the positional arg or add a hidden positional alias.

## 3. Missing CoDocs for lsp package

**Status:** Housekeeping

Neither `cmd/sin-code/internal/lsp/client.go.doc.md` nor
`internal/lsp_cmd.go.doc.md` exist. The lsp package is non-trivial and
warrants a CoDoc.

## 4. LSP testdata script not wired into CI

**Status:** Housekeeping

`cmd/sin-code/testdata/scripts/lsp_live.txt` exists but is not invoked
from the `e2e_test.go` test target (it requires gopls on PATH and a built
`sin-code` binary, so it should be opt-in or behind a build tag).
