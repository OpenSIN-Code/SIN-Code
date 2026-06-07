# `main.go.doc.md` — SIN-Code Unified Binary Entry Point

Entry point for the `sin-code` unified Go binary that replaces 13 standalone tool binaries.

## What it does

- **Registers all subcommands** into a single cobra `rootCmd` (discover, execute, map, grasp, scout, harvest, orchestrate, ibd, poc, sckg, adw, oracle, efm, serve, security, config, self-update, tui).
- **Routes symlinks** — if the binary is invoked via a symlink named after a subcommand (e.g., `discover` → `sin-code discover`), it automatically routes to that subcommand for backwards compatibility.
- **Injects build-time version** into `self-update` via `internal.SetCurrentVersion(Version)`.
- **Runs a non-blocking daily update check** when `--version` or `-v` is used, showing a banner if a newer release is available on GitHub.
- **Delegates execution** to the cobra framework and prints errors via `internal.PrintError`.

## Files that import / touch it

- `cmd/sin-code/internal/*.go` — all subcommand packages are imported and registered here
- `cmd/sin-code/tui.go` — `tuiCmd` is defined in the same package and added to `rootCmd`
- `cmd/sin-code/internal/self-update.go` — receives the build-time version via `SetCurrentVersion`

## Important config values & limits

- **Version injection:** `var Version = "dev"` is overridden at build time via `-ldflags "-X main.Version=..."`. Always set this in CI/release builds.
- **Daily update check:** Stores a timestamp in `~/.config/sin/.last-update-check`. The check is skipped if the file is less than 24 hours old.
- **Update check timeout:** 2 seconds (hardcoded in `checkUpdate()`). If GitHub is slow, the CLI won't hang.

## Symlink routing

When the binary is invoked via a symlink (e.g., `ln -s sin-code discover`), `main()` inspects `filepath.Base(os.Args[0])`. If the basename matches a registered subcommand, it prepends that name to `os.Args` and lets cobra handle the rest. This preserves the old `discover`, `execute`, etc. CLI interfaces after migration to the unified binary.

## Usage examples

```bash
# Build with version
 go build -ldflags "-X main.Version=1.0.4" -o sin-code ./cmd/sin-code

# Create backwards-compatible symlinks
 ln -s sin-code discover
 ln -s sin-code execute
 ln -s sin-code map

# Invoke via symlink (routes automatically)
 ./discover --path . --pattern "*.go"

# Check version (triggers daily update check)
 sin-code --version
```

## Known caveats / footguns

- **Version must be set at build time:** Leaving `Version` as `dev` breaks the update check logic and makes `self-update` always think an update is available.
- **Symlink routing is basename-only:** If the symlink is in a directory with a name that happens to match a subcommand, routing may be incorrect. Use `filepath.Base` to avoid this in most cases, but be aware of edge cases.
- **Daily check uses `os.Args` parsing:** The update check only runs when the first argument is `--version` or `-v`. If you run `sin-code --version --help`, the check still runs because the first arg matches.
- **No config for update check interval:** The 24-hour interval and 2-second timeout are hardcoded. If you need different behavior, you must modify `main.go`.
- **Error handling:** `internal.PrintError` is used for all errors. If `PrintError` is not defined or panics, the CLI will crash after `Execute()` returns an error.
