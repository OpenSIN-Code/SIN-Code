# internal/lsp_cmd.go

What: `sin-code lsp` CLI — thin wrapper around internal/lsp/client.go.
Detects installed LSP servers, spawns them on demand, and exposes the
common LSP operations as subcommands.

Subcommands:
- `sin-code lsp servers` — list detected LSP servers (gopls, pyright, tsserver, etc.)
- `sin-code lsp symbols <file>` — document outline
- `sin-code lsp definition <file> <line> <col>` — go-to-definition
- `sin-code lsp references <file> <line> <col>` — find all references
- `sin-code lsp hover <file> <line> <col>` — type info on hover
- `sin-code lsp rename <file> <line> <col> <new-name>` — rename symbol
- `sin-code lsp format <file>` — apply formatter
- `sin-code lsp diagnostics <file>` — list all errors/warnings

Key dependencies: gopls (Go), pyright (Python), typescript-language-server (TS/JS),
rust-analyzer (Rust). All auto-detected on PATH.

Examples:
```bash
sin-code lsp servers
sin-code lsp definition main.go 5 9
sin-code lsp symbols pkg/foo/bar.go
```
