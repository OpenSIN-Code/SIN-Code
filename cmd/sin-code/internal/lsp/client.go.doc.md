# internal/lsp/client.go

What: JSON-RPC 2.0 over stdio LSP client. Spawns an LSP server (gopls, pyright,
tsserver, rust-analyzer) and communicates via Content-Length framed messages.

Who touches it: lsp_cmd.go (CLI wrapper), lsp test scripts.

Key design decisions:
- **Framing-aware reader** (`readRawLSPFrame`): reads one complete LSP frame
  at a time, tolerating interleaved server-to-client notifications
  (window/logMessage, $/progress, client/registerCapability) that gopls
  v0.20+ emits before the initialize response. This was a real bug
  (st-lsp1) that's now resolved.
- **Notification draining loop** (`readLSPFrame`): loops, calling
  `readRawLSPFrame` and discarding frames with `ID == nil`. Notifications
  are routed to `notificationHandler` if registered.
- **Timeout-bounded reads** (`readFullWithDeadline`): uses a goroutine +
  channel + `time.After` to enforce a deadline on `io.ReadFull`. Without
  this, a frozen LSP server would hang the client forever.
- **Single-threaded per request** (`sync.Mutex` around `nextID`):
  ensures request IDs are monotonically increasing even under concurrent
  `Call()` invocations.

Public API:
- `Start(binary, args, lang, rootURI)` — spawn the server, return *Client
- `(*Client).Call(method, params, result, timeout)` — JSON-RPC request/response
- `(*Client).Notify(method, params)` — fire-and-forget JSON-RPC notification
- `(*Client).SetNotificationHandler(fn)` — register callback for server notifications
- `(*Client).DidOpen/DidChange/DidClose` — document sync notifications
- `(*Client).Definition/References/Hover/Symbols/Rename/Format` — standard LSP operations
- `(*Client).Close()` — graceful shutdown (sends shutdown+exit, waits)

Known limitations:
- Server must support Content-Length framing per LSP spec. gopls, pyright,
  tsserver, rust-analyzer all do.
- Single-server per Client; to use multiple languages simultaneously,
  spawn multiple Clients.
