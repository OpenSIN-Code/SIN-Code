# `self-update.doc.md` — Self-Update Subcommand

Checks GitHub releases for a newer version of sin-code and installs it with automatic backup/restore.

## What it does

- **Queries the GitHub Releases API** for the latest version of `OpenSIN-Code/SIN-Code-Bundle`.
- **Auto-detects platform** (`runtime.GOOS` + `runtime.GOARCH`) to select the correct asset.
- **Downloads and extracts** the correct archive (`.tar.gz` for macOS/Linux, `.zip` for Windows).
- **Backups the current binary** before replacement and restores it if the update fails.
- **Supports `--dry-run`** to check for updates without installing.
- **Supports `--version`** to show current version, platform, and latest release info.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `SelfUpdateCmd` and calls `internal.SetCurrentVersion(Version)` at init time; also calls `internal.CheckUpdateAvailable()` during daily update check
- `cmd/sin-code/main.go` — `main.Version` is passed via `-ldflags` at build time and injected into `self-update` via `SetCurrentVersion`

## Important config values & limits

- **GitHub API timeout:** 30 seconds for release check, 5 minutes for download
- **Daily update check:** Runs only when `--version` or `-v` is used, once per 24 hours (timestamp stored in `~/.config/sin/.last-update-check`)
- **Backup file:** Original binary is renamed to `<binary>.backup` during the update; removed on success, restored on failure
- **Supported platforms:** `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`

## Usage examples

```bash
# Check and install latest stable version
sin-code self-update

# Check only, don't install
sin-code self-update --dry-run

# Show current version and check for updates
sin-code self-update --version

# Build-time version injection (in CI/release pipeline)
go build -ldflags "-X main.Version=1.0.4" -o sin-code ./cmd/sin-code
```

## Known caveats / footguns

- **Version must be set at build time:** If `main.Version` is left as `dev`, self-update will always think an update is available. Always pass `-ldflags "-X main.Version=..."` during release builds.
- **GitHub API rate limits:** Unauthenticated requests to the GitHub API are limited to 60 requests per hour per IP. If you hit the limit, the update check will fail silently (or with an error in `--version` mode).
- **Requires write permission to the binary directory:** If sin-code is installed in a system directory (e.g., `/usr/local/bin/`), the user must have write permissions or run with `sudo`.
- **Windows zip extraction:** Uses Go's `archive/zip` package. Long path names inside the archive may hit Windows MAX_PATH limits on older systems.
- **No downgrade support:** The command always installs the *latest* release. There is no way to pin or downgrade to a specific version via this subcommand.
- **Backup is local only:** If the binary path is on a network mount or ephemeral volume, the backup may not survive a reboot.
