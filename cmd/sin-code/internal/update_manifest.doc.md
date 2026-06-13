# `update_manifest.doc.md` — UpdateManifest Pre/Post Snapshot

Serializable snapshot of all updateable components before and after `sin update`.

## What it does
- **Captures state** of pipx packages, Go binaries, and skills directories as JSON.
- **Persists to** `~/.local/state/sin-code/updates/<ts>/manifest.json` with `0600` perms.
- **Provides round-trip**: `Write(dir)` / `ReadManifest(dir)` with JSON indentation.
- **Used by**: BackupManager (snapshot lifecycle), Rollback (restore pre-state).

## Schema

```json
{
  "timestamp": "20260613T120000Z",
  "sin_code_version": "v3.9.0",
  "go_version": "",
  "os_arch": "",
  "pre": {"pipx_packages": {}, "go_bins": {}, "skills_dirs": {}},
  "post": {},
  "success": false
}
```

## Important config values

- **Snapshot root**: `$SIN_CODE_STATE_ROOT/updates/<ts>/` (default `~/.local/state/sin-code/updates/<ts>/`)
- **Timestamp format**: `20060102T150405Z` (UTC, sortable)
- **File permissions**: `manifest.json` written as `0600` (owner read/write only)

## Usage

```go
m := NewManifest("v3.9.0")
m.Pre = UpdateSnapshot{PipxPackages: map[string]string{"sin-code-bundle": "1.2.0"}}
m.Write(snapshotDir)
loaded, _ := ReadManifest(snapshotDir)
```

## Known caveats

- **No atomic write**: If disk is full mid-write, the manifest is partially written.
  Rollback depends on this file being valid; validate before restoring.
