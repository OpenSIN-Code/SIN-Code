# `efm.doc.md` — Ephemeral Full-Stack Mocking Subcommand

Manages disposable Docker Compose stacks and ephemeral test environments with TTL-based auto-cleanup metadata.

## What it does

- **Lists running Docker containers** via `docker ps` with name, status, ports, and image extraction.
- **Starts stacks** with `docker compose up -d` (falls back to legacy `docker-compose` if needed).
- **Stops stacks** with `docker compose down` (same fallback).
- **Checks stack status** by running `docker compose ps` and reporting `all running`, `partial`, or `no containers running`.
- **Stores TTL metadata** in `~/.local/state/sin-code/efm/` when `--ttl` is set on `up`, recording start time and expiration.
- **Filters services** by stack name when listing after an `up` operation.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `EfmCmd` into the root cobra command
- `cmd/sin-code/internal/efm.go` — self-contained Docker Compose manager
- `cmd/sin-code/internal/execute.go` — shares the `exec.CommandContext` execution pattern and timeout concepts

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--action` | `list` | Action: `up`, `down`, `list`, `status` |
| `--stack` | `""` | Docker Compose file path (required for `up`, `down`, `status`) |
| `--ttl` | `3600` | Time-to-live in seconds (0 = no auto-cleanup metadata) |
| `--format` | `text` | Output: `text` or `json` |

- **Docker CLI required:** `docker` and `docker compose` must be in PATH. No Docker API client library is used.
- **TTL metadata file:** `~/.local/state/sin-code/efm/<stack-basename>.meta` — JSON with `started`, `ttl`, `expires` fields.
- **Auto-cleanup is metadata-only:** The TTL file is written but no background process enforces it. External cron or scheduler must read the metadata and call `efm --action down`.
- **Status output format:** `docker compose ps --format {{.State}}` is used; assumes Docker Compose v2+.

## Usage examples

```bash
# List all running containers
sin-code efm --action list

# Start a stack with 1-hour TTL
sin-code efm --action up --stack docker-compose.yml --ttl 3600

# Check if a stack is running
sin-code efm --action status --stack docker-compose.yml

# Stop and remove a stack
sin-code efm --action down --stack docker-compose.yml

# JSON output for automation
sin-code efm --action up --stack test-env.yml --format json
```

## Known caveats / footguns

- **Docker Compose vs docker-compose fallback:** Tries `docker compose` first, then `docker-compose`. If neither works, the command fails. Ensure Docker Compose v2+ is installed.
- **No auto-cleanup enforcement:** TTL metadata is written but not acted upon. You must run a separate cleanup job (e.g., cron) that reads `.meta` files and calls `efm --action down` for expired stacks.
- **Docker daemon must be running:** If Docker is not running, `docker ps` fails and the error is returned. No automatic Docker start is attempted.
- **Stack file path must be absolute or resolvable:** Relative paths are resolved to absolute before passing to `docker compose -f`. Ensure the file exists before calling `up`/`down`.
- **Container listing is global:** `list` action shows ALL running containers, not just those started by `efm`. Use `--action status --stack` to scope to a specific stack.
- **No port collision detection:** `up` does not check if ports are already in use. Docker Compose will fail with a bind error if ports conflict.
- **JSON output may include raw error strings:** If Docker commands fail, the error text is included in the `error` field of the JSON result.