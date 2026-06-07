# `efm.doc.md` — Ephemeral Full-Stack Mocking Subcommand

Manages disposable Docker Compose / OrbStack stacks and ephemeral test environments with TTL-based auto-cleanup metadata.

## What it does

- **Lists running containers** via `<runtime> ps` with name, status, ports, and image extraction.
- **Starts stacks** with `<runtime> compose up -d` (falls back to legacy `<runtime>-compose` if needed).
- **Stops stacks** with `<runtime> compose down` (same fallback).
- **Checks stack status** by running `<runtime> compose ps` and reporting `all running`, `partial`, or `no containers running`.
- **Stores TTL metadata** in `~/.local/state/sin-code/efm/` when `--ttl` is set on `up`, recording start time, expiration, and the runtime used.
- **Filters services** by stack name when listing after an `up` operation.

## Container runtime

The `--runtime` flag (default `auto`) selects which container CLI to invoke:

| Value     | Behavior |
|-----------|----------|
| `auto`    | On macOS, use `orb` (OrbStack) if available, else `docker`. On Linux, use `docker`. |
| `orb`     | Force OrbStack (`orb compose …`, fallback `orb-compose`). |
| `docker`  | Force Docker (`docker compose …`, fallback `docker-compose`). |

OrbStack is a Docker CLI-compatible runtime for macOS that uses native virtualization
instead of Docker Desktop's Linux VM. `orb` accepts the same `compose`, `ps`, `up`, `down`
subcommands as `docker`, so no other code paths need to branch on the runtime.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `EfmCmd` into the root cobra command
- `cmd/sin-code/internal/efm.go` — self-contained container-runtime manager
- `cmd/sin-code/internal/execute.go` — shares the `exec.CommandContext` execution pattern and timeout concepts

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--action` | `list` | Action: `up`, `down`, `list`, `status` |
| `--stack` | `""` | Docker Compose file path (required for `up`, `down`, `status`) |
| `--ttl` | `3600` | Time-to-live in seconds (0 = no auto-cleanup metadata) |
| `--format` | `text` | Output: `text` or `json` |
| `--runtime` | `auto` | Container runtime: `auto`, `orb`, `docker` |

- **Container CLI required:** `docker`, `docker compose`, `orb`, `orb-compose`, or the legacy `docker-compose` must be in PATH. No Docker API client library is used.
- **TTL metadata file:** `~/.local/state/sin-code/efm/<stack-basename>.meta` — JSON with `started`, `ttl`, `expires`, and `runtime` fields.
- **Auto-cleanup is metadata-only:** The TTL file is written but no background process enforces it. External cron or scheduler must read the metadata and call `efm --action down`.
- **Status output format:** `<runtime> compose ps --format {{.State}}` is used; assumes Compose v2+.

## Usage examples

```bash
# Auto-detect runtime (OrbStack on macOS, Docker on Linux)
sin-code efm --action list

# Start a stack with 1-hour TTL
sin-code efm --action up --stack docker-compose.yml --ttl 3600

# Check if a stack is running
sin-code efm --action status --stack docker-compose.yml

# Stop and remove a stack
sin-code efm --action down --stack docker-compose.yml

# Force OrbStack even if Docker is also installed
sin-code efm --action list --runtime orb

# Force plain Docker (skip OrbStack auto-detect)
sin-code efm --action list --runtime docker

# JSON output for automation
sin-code efm --action up --stack test-env.yml --format json
```

## Known caveats / footguns

- **Compose vs legacy fallback:** Tries `<runtime> compose` first, then `<runtime>-compose` (or `docker-compose` for empty runtime). If none work, the command fails.
- **Auto-detect prefers OrbStack on macOS** when both runtimes are installed. Use `--runtime docker` to force the legacy path.
- **No auto-cleanup enforcement:** TTL metadata is written but not acted upon. You must run a separate cleanup job (e.g., cron) that reads `.meta` files and calls `efm --action down` for expired stacks.
- **Daemon must be running:** If the chosen runtime's daemon is not running, `<runtime> ps` fails and the error is returned. No automatic daemon start is attempted.
- **Stack file path must be absolute or resolvable:** Relative paths are resolved to absolute before passing to `<runtime> compose -f`. Ensure the file exists before calling `up`/`down`.
- **Container listing is global:** `list` action shows ALL running containers, not just those started by `efm`. Use `--action status --stack` to scope to a specific stack.
- **No port collision detection:** `up` does not check if ports are already in use. Compose will fail with a bind error if ports conflict.
- **JSON output may include raw error strings:** If runtime commands fail, the error text is included in the `error` field of the JSON result.
- **Runtime field is reported in JSON output** for every action, so downstream automation can verify which CLI was actually used.
