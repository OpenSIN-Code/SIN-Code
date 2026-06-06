# SIN-Code Desktop

Native desktop GUI for the SIN-Code agent engineering stack.

**Stack:**
- **Backend**: Rust (Tauri v2)
- **Frontend**: Next.js 14 (static export)
- **Design tokens**: Shared with the Go-based TUI (`internal/tui/theme/tokens.go`)

## Architecture

```
SIN-Code Desktop (Tauri v2)
├── src-tauri/          # Rust backend
│   ├── Cargo.toml      # Dependencies (tauri, plugins, tokio, tracing)
│   ├── tauri.conf.json # Window, tray, security, bundle config
│   ├── icons/          # Tray + app icons
│   └── src/main.rs     # Commands: greet, get_version, check_sin_cli, run_sin_command
└── web/                # Next.js frontend
    ├── src/app/        # App Router (page.tsx, layout.tsx, globals.css)
    ├── src/components/ # Sidebar, Dashboard, CodeHub, SecurityPanel, TuiLauncher, Settings, StatusBar
    └── tailwind.config.ts # Wired to shared design tokens via CSS variables
```

## Design system: shared with TUI

The same color story, spacing, and typography is used across:
- **TUI** (Go + Lipgloss): `internal/tui/theme/tokens.go`
- **GUI** (Next.js + Tailwind): `web/theme/tokens.css` (imported into `globals.css`)
- **Future mobile / IDE plugins** can re-use the same tokens

When you change a token, both UIs update automatically.

## Prerequisites

```bash
# macOS
xcode-select --install
brew install rust node pnpm

# Tauri CLI (one-time)
cargo install tauri-cli --version "^2"
```

## Development

```bash
cd desktop

# Install web dependencies
cd web && pnpm install && cd ..

# Run in dev mode (Tauri spawns the Next.js dev server automatically)
cd src-tauri && cargo tauri dev
```

## Production build

```bash
cd src-tauri
cargo tauri build
# Output: src-tauri/target/release/bundle/dmg/SIN-Code Desktop_1.0.0_aarch64.dmg
#         src-tauri/target/release/bundle/macos/SIN-Code Desktop.app
```

## Commands (Tauri ↔ Rust)

| Command | Description |
|---------|-------------|
| `get_version()` | Returns app version from Cargo.toml |
| `check_sin_cli()` | Checks if `sin` is on PATH |
| `run_sin_command(args)` | Forwards args to `sin` binary, returns stdout |
| `greet(name)` | Demo command (Hello world) |

## Tauri plugins enabled

- `store` — persistent key-value config
- `opener` — open URLs/files in default app
- `clipboard-manager` — copy/paste
- `dialog` — native file/folder pickers
- `fs` — filesystem access
- `http` — HTTP client
- `notification` — system notifications
- `os` — OS info (version, arch)
- `process` — process control
- `shell` — execute commands
- `updater` — auto-update (disabled in dev)
- `webview-window` — manage windows

## Security

- CSP enabled: `default-src 'self' data: blob: https:`
- DevTools only in debug builds
- `window.open()` blocked by default
- `webview-data-url` only allowed in dev

## Build size

| Platform | Binary | Bundle |
|----------|--------|--------|
| macOS (arm64) | ~6 MB | ~15 MB (.app) |
| macOS (x86_64) | ~6 MB | ~15 MB (.app) |
| Windows | ~7 MB | ~25 MB (.msi) |
| Linux | ~8 MB | ~35 MB (.AppImage) |

## Versioning

Desktop version tracks the Bundle version. Currently **v1.0.0** (synchronized with Bundle v1.3.0+).

## Related

- [SIN-Code-Bundle](../../) — the parent meta-CLI (`sin` command)
- [internal/tui](../../internal/tui/) — the Go-based TUI sibling
- [web/theme/tokens.css](../theme/tokens.css) — shared design tokens
- [OpenSIN-Code org](https://github.com/OpenSIN-Code)
