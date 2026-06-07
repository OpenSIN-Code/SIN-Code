# sin-code todo v2 — Design Document

**Status:** Phase 1 (core CRUD + bbolt storage) implemented. Phase 2 (notifications, TUI integration, hooks) planned.

## 1. Overview

`sin-code todo` is the SIN-Code issue tracker, built into the unified `sin-code` binary. It replaces the legacy `sin-code orchestrate` command with a SOTA, Beads-inspired issue tracker that works entirely offline, stores data locally in a single bbolt file, and integrates with the agent ecosystem via notifications and hooks.

### Design Goals

- **Offline-first:** no network calls, no external services, no database server
- **Agent-native:** the same data model works for humans (CLI), agents (MCP), and dashboards (TUI)
- **Append-only audit log:** every state change is recorded with actor + timestamp
- **Dependency graph:** block/parent-child/related edges with cycle detection
- **Compaction:** old closed todos are summarized to save context
- **Project namespaces:** multi-repo workflows without ID collisions
- **Backwards compatible:** `sin-code orchestrate` remains as an alias

## 2. Feature Comparison

| Feature | `sin-code orchestrate` (legacy) | `sin-code todo` (v2) | `bd` (beads) | opencode todo |
|---|---|---|---|---|
| Storage | JSON file | bbolt | Dolt (MySQL-compatible) | JSON file |
| Hash IDs | ❌ (auto-increment int) | ✅ `st-{4chars}` | ✅ `bd-{n}` | ❌ |
| Dependencies | ✅ (int IDs) | ✅ (typed edges) | ✅ | ❌ |
| Cycle detection | ❌ | ✅ | ✅ | ❌ |
| Audit log | ❌ | ✅ (append-only) | ✅ | ❌ |
| Ready/Blocked queries | ❌ | ✅ | ✅ | ❌ |
| Project namespaces | ❌ | ✅ | ✅ (via prefix) | ❌ |
| Compaction | ❌ | ✅ (summarize old) | ✅ | ❌ |
| Search | ❌ | ✅ (title/description) | ✅ | ❌ |
| JSON/Markdown export | ❌ | ✅ (3 formats) | ✅ | ❌ |
| MCP tool | ❌ | ✅ (via `sin_todo`) | ❌ | ❌ |
| Persistent memory | ❌ | ✅ (`remember` cmd) | ✅ | ❌ |
| Prime context | ❌ | ✅ (`prime` cmd) | ✅ | ❌ |
| Hooks | ❌ | 🔜 planned | ✅ | ❌ |
| Notifications | ❌ | 🔜 planned | ❌ | ✅ (in TUI) |

## 3. Data Model

### 3.1 `Todo` struct

```go
type Todo struct {
    ID          string     `json:"id"`                    // hash: st-{4chars}
    Title       string     `json:"title"`                 // required
    Description string     `json:"description,omitempty"` // optional, multi-line
    Status      Status     `json:"status"`                // open, in_progress, done, cancelled, blocked
    Priority    Priority   `json:"priority"`              // P0, P1, P2, P3
    Type        TodoType   `json:"type"`                  // task, bug, feature, chore, epic, question
    Tags        []string   `json:"tags,omitempty"`
    Assignee    string     `json:"assignee,omitempty"`
    Parent      string     `json:"parent,omitempty"`
    ExternalRef string     `json:"external_ref,omitempty"`
    Project     string     `json:"project,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
    ClosedAt    *time.Time `json:"closed_at,omitempty"`
    DueAt       *time.Time `json:"due_at,omitempty"`
    Estimate    int        `json:"estimate_minutes,omitempty"`
    Notes       string     `json:"notes,omitempty"`
    Compacted   bool       `json:"compacted,omitempty"`
    Summary     string     `json:"summary,omitempty"`     // set by Compact
}
```

### 3.2 `Dependency` struct

```go
type Dependency struct {
    From string `json:"from"`  // child todo ID
    To   string `json:"to"`    // parent (blocker) todo ID
    Type DepType `json:"type"` // blocks, parent-child, related, discovered-from, duplicates, supersedes
}
```

Only `blocks` edges are considered for the Ready/Blocked queries. Other types are metadata.

### 3.3 `AuditEntry` struct

```go
type AuditEntry struct {
    ID        string    `json:"id"`        // au-{6hex}
    TodoID    string    `json:"todo_id"`
    Timestamp time.Time `json:"timestamp"`
    Actor     string    `json:"actor"`     // git user.name or --as
    Action    string    `json:"action"`    // create, update, claim, complete, cancel, delete, dep:add, dep:remove
    From      string    `json:"from,omitempty"`
    To        string    `json:"to,omitempty"`
    Note      string    `json:"note,omitempty"`
}
```

### 3.4 `Memory` struct (persistent project memory)

```go
type Memory struct {
    ID        string    `json:"id"`
    Insight   string    `json:"insight"`
    CreatedAt time.Time `json:"created_at"`
    Actor     string    `json:"actor"`
}
```

## 4. Storage

Single bbolt database at `~/.config/sin-code/todo.db` (overridable via `--db`).

### Buckets

| Bucket | Purpose | Key format | Value |
|---|---|---|---|
| `todos` | All todos | `{id}` | JSON `Todo` |
| `deps` | Dependency edges | `{from}\x00{to}\x00{type}` | `"1"` |
| `audit` | Append-only audit log | `{unixnano}\x00{id}` | JSON `AuditEntry` |
| `memories` | Project memory | `{unixnano}\x00{id}` | JSON `Memory` |
| `meta` | Key-value metadata | `{key}` | `{value}` |
| `idx_status` | Status → todo IDs | `{status}\x00{id}` | `""` |
| `idx_priority` | Priority → todo IDs | `{priority}\x00{id}` | `""` |
| `idx_assignee` | Assignee → todo IDs | `{assignee}\x00{id}` | `""` |
| `idx_project` | Project → todo IDs | `{project}\x00{id}` | `""` |
| `idx_tag` | Tag → todo IDs | `{tag}\x00{id}` | `""` |

Secondary indexes enable fast filtered queries without scanning the `todos` bucket.

## 5. ID Generation

Hash-based IDs prevent collisions in multi-agent, multi-repo scenarios.

```go
const idPrefix = "st-"
const idBodyLen = 4
const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func GenerateID() string {
    h := sha1.Sum([]byte(fmt.Sprintf("%d-%d-%d", time.Now().UnixNano(), idSalt, i)))
    body := encodeBase36(uint64(h[0])<<24|uint64(h[1])<<16|uint64(h[2])<<8|uint64(h[3]), 4)
    return "st-" + body
}
```

- Format: `st-{4chars}` (7 chars total)
- Alphabet: base36 (digits + lowercase)
- Collision check: 32 retries against in-process `seenIDs` map
- Fallback: 8-char timestamp suffix if all retries collide

## 6. CLI Subcommands (27 total)

### Core CRUD
- `add` — create with --title, --desc, --priority, --type, --tags, --assignee, --parent, --external-ref, --project
- `list` — filter by --status, --priority, --type, --tag, --assignee, --project, --search; --all for no filter
- `show <id>` — full details + audit log + dependencies
- `update <id>` — update any field
- `claim <id>` — atomically claim (sets assignee, status=in_progress)
- `unclaim <id>`
- `complete <id>` — mark done
- `cancel <id>` — mark cancelled
- `delete <id>` — soft (default) or hard (--soft=false)

### Dependencies
- `dep add <child> <parent> --type blocks` — link
- `dep remove <child> <parent>`
- `deps <id>` — show dependency tree

### Queries
- `ready` — list unblocked open work (P0 first)
- `blocked` — list blocked work
- `search <query>` — substring search
- `graph` — DOT output for Graphviz
- `stats` — counts by status/priority/type/assignee
- `timeline [id]` — audit log (optionally for a specific todo)
- `mine` — assigned to current user

### Project & Memory
- `project [name]` — switch project namespace
- `remember <insight>` — store persistent memory
- `prime` — print context for agent prompt
- `compact` — summarize old closed todos

### Housekeeping
- `init` — initialize bbolt DB
- `doctor` — health check
- `export` — JSON/JSONL/Markdown
- `import` — JSON/JSONL

## 7. Backward Compatibility

The legacy `sin-code orchestrate` command remains available as a thin wrapper. It maps:
- `orchestrate --action add --title "X"` → `todo add --title "X"`
- `orchestrate --action list` → `todo list`
- `orchestrate --action complete --id 1` → `todo complete st-0001` (if ID matches) or lookup by ordinal

The legacy `~/.local/state/sin-code/orchestrate.json` file is auto-migrated to bbolt on first `todo init`.

## 8. Testing

- **50+ unit tests** in `todo_test.go` covering Store CRUD, ID generation, filtering, cycle detection, audit log, compaction, memory, queries
- **5 E2E testscript tests** in `testdata/scripts/todo_*.txt` covering CLI lifecycle, ready, stats, graph
- **Coverage:** 46.9% of statements (Store + query logic 100%, command handlers not directly tested via cobra)

## 9. Roadmap

### Phase 1 (DONE) — Core
- [x] Data model (Todo, Dependency, AuditEntry, Memory)
- [x] bbolt storage with secondary indexes
- [x] Hash ID generation
- [x] 27 CLI subcommands
- [x] Dependency cycle detection
- [x] Audit log
- [x] Compaction
- [x] Export/Import (JSON, JSONL, Markdown)
- [x] Project namespaces
- [x] Unit tests (50+)
- [x] E2E testscripts (5)

### Phase 2 (PLANNED) — Notifications
- [ ] `internal/notifications` package with bbolt store
- [ ] TUI channel for live banner
- [ ] macOS notification via `osascript`
- [ ] Webhook delivery
- [ ] `sin-code notifications list/read/dismiss/listen/clear`
- [ ] MCP `sin_notifications` tool

### Phase 3 (PLANNED) — TUI Integration
- [ ] "Todos" view as 5th tab in TUI
- [ ] Sidebar badge: "🔵 N open 🟡 N blocked 🔴 N overdue"
- [ ] Footer counter
- [ ] Notification banner at top

### Phase 4 (PLANNED) — Hooks
- [ ] `~/.config/sin-code/hooks.toml` config
- [ ] pre_add, post_add, pre_complete, post_complete, etc.
- [ ] Shell command execution with `SIN_TODO_*` env vars
- [ ] `sin-code todo hook list/add/remove/test`

## 10. File Structure

```
cmd/sin-code/internal/todo/
├── model.go       (161 lines) — Todo, Dependency, AuditEntry, Memory, enums
├── store.go       (382 lines) — bbolt Store wrapper
├── id.go          (82 lines)  — hash ID generation
├── deps.go        (180 lines) — dependency CRUD + cycle detection
├── query.go       (182 lines) — ListFiltered, Ready, Blocked, Search, Mine, Stats
├── audit.go       (75 lines)  — AppendAudit, ListAudit, CountAudit
├── compact.go     (87 lines)  — Compact + summarize
├── remember.go    (62 lines)  — AddMemory, ListMemories
├── todo.go        (1290 lines) — all cobra commands
└── todo_test.go   (817 lines) — 50+ tests
```

## 11. Usage Examples

```bash
# Add a P0 bug
sin-code todo add --title "Fix auth bypass" --priority P0 --type bug --tags "security,urgent"

# List unblocked P0 work
sin-code todo ready

# Add a dependency
sin-code todo dep add st-child st-parent --type blocks

# Mark done
sin-code todo complete st-child

# View all open work as JSON (for agents)
sin-code todo ready --format json

# Compact old closed todos
sin-code todo compact --older-than 720h

# Export for backup
sin-code todo export --format json --output backup.json

# Switch project namespace
sin-code todo project myorg/myrepo

# Store persistent memory
sin-code todo remember "Always run govulncheck before tagging releases"

# Print context for agent prompt
sin-code todo prime
```

## 12. Inspiration & References

- **[beads](https://github.com/gastownhall/beads)** — Dolt-backed issue tracker for AI agents
- **[bd CLI](https://github.com/gastownhall/beads)** — reference implementation
- **[TaskMaster](https://github.com/eyaltoledano/claude-task-master)** — Claude Code task management
- **[opencode todo](https://opencode.ai)** — inline todo UX in the TUI

## 13. License

MIT — see [LICENSE](../../LICENSE).
