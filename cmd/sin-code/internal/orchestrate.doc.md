# `orchestrate.doc.md` — Task Management Subcommand

Manages tasks with JSON file storage, CRUD operations, tags, dependencies, blocker detection, and rollback plans.

## What it does

- **Stores tasks** in `~/.local/state/sin-code/orchestrate.json` as a simple JSON file with auto-incrementing IDs.
- **CRUD operations:** `add`, `remove`, `complete`, `status`, `list`.
- **Tags support:** Comma-separated tags stored as arrays; filterable in list view.
- **Dependency tracking:** Task struct includes `Dependencies` (task IDs) and `Blockers` (text descriptions) for future dependency-graph resolution.
- **Rollback field:** Each task has a `rollback` string field for storing rollback plans (manual entry; no automatic execution).
- **Status icons:** Text output uses `○` pending, `●` in-progress, `✗` blocked, `✓` completed.
- **List sorting:** Pending → in-progress → blocked → completed.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `OrchestrateCmd` into the root cobra command
- `cmd/sin-code/internal/orchestrate.go` — self-contained task manager

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--action` | `list` | Action: `add`, `remove`, `list`, `status`, `complete` |
| `--title` | `""` | Task title (required for `add`) |
| `--tags` | `""` | Comma-separated tags |
| `--id` | `""` | Task ID (required for `remove`, `complete`, `status`) |
| `--format` | `text` | Output: `text` or `json` |

- **State file:** `~/.local/state/sin-code/orchestrate.json`
- **Versioning:** State file has `version` field (currently 1) for future migration support.
- **No automatic dependency resolution:** `Dependencies` and `Blocked` fields are stored but not enforced. Completing a task does not auto-unblock dependents.

## Usage examples

```bash
# Add a new task
sin-code orchestrate --action add --title "Implement feature X" --tags "urgent,backend"

# List all tasks
sin-code orchestrate --action list

# Mark task #1 as completed
sin-code orchestrate --action complete --id 1

# Get task status as JSON
sin-code orchestrate --action status --id 1 --format json

# Remove a task
sin-code orchestrate --action remove --id 2
```

## Known caveats / footguns

- **No concurrency safety:** The JSON file is read and written without file locking. Running multiple `sin-code orchestrate` commands simultaneously may corrupt the state file.
- **No dependency validation:** The `Dependencies` field stores integers but does not validate that they exist or prevent circular dependencies.
- **ID is integer but parsed loosely:** `parseID` uses `fmt.Sscanf` — non-numeric IDs silently become 0, which may match task #0 (doesn't exist) and fail confusingly.
- **State file is not backed up:** If corrupted, there is no automatic recovery. Manual backup recommended before batch edits.
- **Rollback field is manual only:** It stores a text description but does not execute anything. Use it as a checklist, not an automation tool.