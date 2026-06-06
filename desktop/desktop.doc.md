# desktop — SIN-Code Desktop GUI (Tauri v2 + Next.js)

## What
Native desktop application for the SIN-Code agent engineering stack. Provides a
GUI alternative to the TUI (Bubbletea) and CLI (`sin`).

## Stack
- **Backend**: Rust + Tauri v2 (`desktop/src-tauri/`)
- **Frontend**: Next.js 14 (App Router, static export) (`desktop/web/`)
- **Design system**: shared tokens (`web/theme/tokens.css` ↔ `internal/tui/theme/tokens.go`)

## Key files
- `desktop/src-tauri/Cargo.toml` — Rust dependencies
- `desktop/src-tauri/tauri.conf.json` — Tauri config (windows, tray, plugins, bundle)
- `desktop/src-tauri/src/main.rs` — Tauri commands + tray setup
- `desktop/scripts/build-icons.sh` — PNG/ICNS/ICO generator from `icon.svg`
- `desktop/web/src/app/page.tsx` — main app shell (Sidebar + 5 views + StatusBar)
- `desktop/web/src/components/` — Sidebar, Dashboard, CodeHub, SecurityPanel, TuiLauncher, Settings, StatusBar
- `desktop/web/src/app/globals.css` — imports `tokens.css`, Tailwind directives
- `desktop/web/tailwind.config.ts` — exposes CSS variables as Tailwind utilities

## Tauri commands
| Command | Description |
|---------|-------------|
| `get_version()` | Returns app version from Cargo.toml |
| `check_sin_cli()` | Checks if `sin` is on PATH |
| `run_sin_command(args)` | Forwards args to `sin`, returns stdout |
| `greet(name)` | Demo command |

## Views (frontend)
- **Dashboard** — system status + quick actions
- **Code Hub** — run IBD, PoC, ADW, Oracle, SCKG, CoDocs
- **Security** — run any of the 8 security tools (secrets, sast, sca, sbom, container, iac, license, dast)
- **TUI** — instructions for launching the Bubbletea TUI from a real terminal
- **Settings** — theme, auto-update preferences

## How to develop
```bash
cd desktop
cd web && pnpm install && cd ..
cd src-tauri && cargo tauri dev
```

## How to build (production)
```bash
cd desktop/src-tauri
cargo tauri build
# Output: src-tauri/target/release/bundle/{dmg,macos,msi,deb,appimage}/
```

## Design system integration
The Tailwind config (`desktop/web/tailwind.config.ts`) reads CSS variables
defined in `web/theme/tokens.css`. The same tokens are mirrored in
`internal/tui/theme/tokens.go` for the Go-based TUI. **One source of truth, two
runtimes.**

## Related
- [`../internal/tui/`](../internal/tui/) — Go-based TUI sibling
- [`../web/theme/tokens.css`](../web/theme/tokens.css) — shared design tokens
- [`../src/sin_code_bundle/cli.py`](../src/sin_code_bundle/cli.py) — the underlying Python CLI

## Known caveats
- The TUI cannot run inside the desktop webview (no real TTY). Users must launch
  `sin tui` from their system terminal — see the TuiLauncher view for instructions.
- Icons need to be generated from `icon.svg` via `scripts/build-icons.sh` before
  the first `cargo tauri build`. Tauri will fail without `icon.icns` / `icon.ico`.
- Tauri v2 requires Rust 1.77+; macOS needs 13.0+, Windows 10+, Ubuntu 22.04+.
