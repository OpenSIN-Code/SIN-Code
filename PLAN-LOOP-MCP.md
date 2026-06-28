# PLAN: sin_run_loop + sin_goal_queue MCP Tools

**Repo:** SIN-Code (`/Users/jeremy/dev/SIN-Code`)
**Goal:** Expose the full verified agent loop and the autonomous goal queue as MCP tools so opencode (and any MCP client) can delegate complete tasks instead of writing 100 prompts in sequence.

---

## Architecture

```
opencode (or any MCP client)
  │  calls MCP tool
  ▼
sin-code serve (MCP server, 44+ tools)
  ├── sin_run_loop (NEW — Option A)
  │     prompt → loopbuilder.Build() → loop.Run() → {summary, verified, turns, session_id}
  │     SYNCHRONOUS: blocks until the loop finishes (PLAN→ACT→VERIFY→DONE)
  │     Full stack: Verify-Gate, Stop-Gate, Lessons, Compaction, Loop-Detection, Fusion
  │
  ├── sin_goal_add (NEW — Option C)
  │     prompt → autonomy.Queue.Add() → goal_id
  │     ASYNCHRONOUS: enqueues and returns immediately
  │     Daemon picks it up and runs autonomously
  │
  ├── sin_goal_list (NEW — Option C)
  │     → autonomy.Queue.List() → [{id, status, prompt, attempts, ...}]
  │
  ├── sin_goal_status (NEW — Option C)
  │     id → autonomy.Queue.Get() + Children() → {goal, children}
  │
  └── sin_goal_complete (NEW — Option C)
        id → autonomy.Queue.Complete() → ok
```

---

## Wave 1: sin_run_loop (Option A — Synchronous Loop Delegation)

### Task 1.1: `sin_run_loop` MCP tool registration

**File:** `cmd/sin-code/internal/serve.go`

Add a new tool spec to the `tools` slice:

```go
{
    name:        "sin_run_loop",
    description: "Run a prompt through the full SIN-Code agent loop (PLAN→ACT→VERIFY→DONE). Returns {session_id, summary, verified, turns}. Blocks until completion. Includes Verify-Gate, Stop-Gate, Lessons, Compaction, Loop-Detection. This is the synchronous delegation path — one call, one verified task.",
    handler:     handleRunLoop,
    schema: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "prompt":     map[string]any{"type": "string"},
            "workspace":  map[string]any{"type": "string", "default": "."},
            "model":      map[string]any{"type": "string"},
            "max_turns":  map[string]any{"type": "integer", "default": 80},
            "verify_cmd": map[string]any{"type": "string"},
            "yolo":       map[string]any{"type": "boolean", "default": false},
            "agent":      map[string]any{"type": "string"},
            "style":      map[string]any{"type": "string", "enum": []string{"default", "verbose", "normal", "terse", "ultra"}, "default": "default"},
            "criteria":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
        },
        "required": []string{"prompt"},
    },
},
```

**Acceptance criteria:**
- `sin-code serve` lists `sin_run_loop` in the tool manifest
- Tool spec has correct JSON schema
- `grep -c "sin_run_loop" cmd/sin-code/internal/serve.go` returns ≥ 2 (spec + handler ref)

### Task 1.2: `handleRunLoop` handler implementation

**File:** `cmd/sin-code/internal/serve_extra_handlers.go` (or new `serve_loop_handler.go`)

The handler must:
1. Parse args: `prompt` (required), `workspace` (default `.`), `model`, `max_turns` (default 80), `verify_cmd`, `yolo`, `agent`, `style`, `criteria[]`
2. Resolve workspace to absolute path
3. Build a `loopbuilder.Config` with:
   - `Headless: true` (M4: ask → deny)
   - `VerifyMode: "poc"` (default; `verify_cmd` overrides)
   - `MaxTurns: max_turns`
   - `Yolo: yolo` (skips permission engine)
   - `Model: model` (if provided)
   - `Style: style`
   - `AgentName: agent` (if provided)
   - `Contract: &goalcontract.GoalContract{SemanticCriteria: criteria}` (if criteria provided → activates stop-gate)
4. Call `loopbuilder.Build(ctx, cfg, memStore)` → get `*agentloop.Loop`
5. Create or resume a session via `session.Store`
6. Call `loop.Run(ctx, sess, prompt)`
7. Return JSON: `{"session_id": "...", "summary": "...", "verified": true/false, "turns": N}`

**Timeout:** The MCP handler needs a longer timeout than the default 5 minutes in `runSinCodeCLI`. The loop itself manages its own timeout via `MaxTurns`. The handler should use `context.WithCancel` and let the loop's internal turn-budget be the bound. No `runSinCodeCLI` — this is an IN-PROCESS call, not a subprocess.

**Key difference from `handleOrchestratorRun`:** `handleOrchestratorRun` shells out to `sin-code orchestrator-run` via `runSinCodeCLI` (subprocess, 5min cap). `handleRunLoop` builds the loop IN-PROCESS via `loopbuilder.Build()` and runs it directly. This is critical because:
- The loop needs access to the workspace filesystem
- The loop runs for potentially 80+ turns (minutes to hours)
- Subprocess overhead is wasteful when we're already in the `sin-code serve` process
- The loop's hooks, lessons, and memory stores are already initialized in the serve process

**Acceptance criteria:**
- `handleRunLoop` is a `func(ctx context.Context, args map[string]any) (string, error)`
- Returns valid JSON with `session_id`, `summary`, `verified`, `turns` fields
- When `prompt` is empty, returns error `"prompt is required"`
- When loop completes with `verify_mode != "off"` and gate passes, `verified: true`
- When loop completes but gate fails, `verified: false`
- Handler does NOT use `runSinCodeCLI` — it calls `loopbuilder.Build` directly
- `go build ./cmd/sin-code/...` passes
- `go vet ./cmd/sin-code/...` passes

### Task 1.3: Permission defaults for `sin_run_loop`

**File:** `cmd/sin-code/internal/permission_defaults.go`

Add to the default permission rules:

```go
{Pattern: "sin_run_loop", Policy: "ask", Layer: "agentloop", Reason: "spawns full agent loop with tool access — operator confirmation required (M4)"},
```

Policy is `ask` (not `allow`) because `sin_run_loop` spawns a full agent loop that can read/write files, execute commands, and make API calls. In headless mode, `ask` resolves to `deny` unless `--yolo` is set.

**Acceptance criteria:**
- `grep "sin_run_loop" cmd/sin-code/internal/permission_defaults.go` returns a match
- Policy is `ask`, not `allow`
- `go test ./cmd/sin-code/internal/... -run TestPermissionDefaults` passes

### Task 1.4: Integration test

**File:** `cmd/sin-code/internal/serve_loop_handler_test.go`

Test cases:
1. `sin_run_loop` with empty prompt → error
2. `sin_run_loop` with a simple prompt in a temp workspace → returns JSON with `session_id`, `verified` field
3. `sin_run_loop` with `criteria` → stop-gate is activated (contract non-nil)
4. `sin_run_loop` with `yolo: true` → permission engine allows all

Mock the LLM provider with a stub that returns a no-op completion (no tool calls) so the loop terminates in 1 turn.

**Acceptance criteria:**
- `go test ./cmd/sin-code/internal/ -run TestRunLoop -race -count=1` passes
- Test verifies JSON output schema
- Test is race-clean (`-race` flag)

---

## Wave 2: sin_goal_* MCP tools (Option C — Asynchronous Goal Queue)

### Task 2.1: `sin_goal_add` MCP tool

**File:** `cmd/sin-code/internal/serve.go` (spec) + `serve_extra_handlers.go` (handler)

```go
{
    name:        "sin_goal_add",
    description: "Enqueue a goal for autonomous execution by the sin-code daemon. Returns immediately with the goal ID. The daemon will pick it up, run the full agent loop (with Verify-Gate, Stop-Gate, Lessons), and mark it verified/failed. Use sin_goal_status to poll progress.",
    handler:     handleGoalAdd,
    schema: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "prompt":    map[string]any{"type": "string"},
            "workspace": map[string]any{"type": "string", "default": "."},
            "priority":  map[string]any{"type": "integer", "default": 0},
            "retries":   map[string]any{"type": "integer", "default": 3},
            "criteria":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
        },
        "required": []string{"prompt"},
    },
},
```

Handler: calls `autonomy.Open(autonomy.DefaultPath())` → `q.Add(ctx, prompt, ws, priority, retries)` → returns `{"goal_id": N, "status": "pending"}`

**Acceptance criteria:**
- `sin-goal-add` with a prompt → returns JSON with `goal_id`
- Goal appears in `sin-code goal list`
- `go build ./cmd/sin-code/...` passes

### Task 2.2: `sin_goal_list` MCP tool

Handler: calls `q.List(ctx, status)` → returns JSON array of goals.

Schema: `{"status": {"type": "string", "enum": ["pending", "running", "verified", "failed", "exhausted", ""]}, "format": {"type": "string", "enum": ["text", "json"], "default": "json"}}`

**Acceptance criteria:**
- Returns JSON array with goals
- `status` filter works
- Empty queue returns `[]`

### Task 2.3: `sin_goal_status` MCP tool

Handler: calls `q.Get(ctx, id)` + `q.Children(ctx, id)` → returns `{"goal": {...}, "children": [...]}`

Schema: `{"id": {"type": "string"}, "format": {"type": "string", "enum": ["text", "json"], "default": "json"}}`

**Acceptance criteria:**
- Returns goal details + subtasks
- Non-existent ID → error

### Task 2.4: `sin_goal_complete` MCP tool

Handler: calls `q.Complete(ctx, id, session)` → returns `{"ok": true}`

Schema: `{"id": {"type": "string"}, "session": {"type": "string"}}`

**Acceptance criteria:**
- Goal status changes to `verified`
- Non-existent ID → error

### Task 2.5: Permission defaults for `sin_goal_*`

**File:** `cmd/sin-code/internal/permission_defaults.go`

```go
{Pattern: "sin_goal_add",      Policy: "ask",    Layer: "autonomy", Reason: "enqueues autonomous work — operator confirmation (M4)"},
{Pattern: "sin_goal_list",     Policy: "allow",  Layer: "autonomy", Reason: "read-only goal listing"},
{Pattern: "sin_goal_status",   Policy: "allow",  Layer: "autonomy", Reason: "read-only goal status"},
{Pattern: "sin_goal_complete", Policy: "ask",    Layer: "autonomy", Reason: "marks goal done — operator confirmation (M4)"},
```

**Acceptance criteria:**
- `sin_goal_list` and `sin_goal_status` are `allow` (read-only)
- `sin_goal_add` and `sin_goal_complete` are `ask` (mutating)

### Task 2.6: Integration tests for goal tools

**File:** `cmd/sin-code/internal/serve_goal_handler_test.go`

Test cases:
1. `sin_goal_add` → returns `goal_id` ≥ 1
2. `sin_goal_list` → array contains the goal from step 1
3. `sin_goal_status` with the goal ID → goal details
4. `sin_goal_complete` → goal status = `verified`
5. `sin_goal_add` with `criteria` → goal has contract attached

Use a temp SQLite DB for the goal queue (override `autonomy.DefaultPath()` in tests).

**Acceptance criteria:**
- `go test ./cmd/sin-code/internal/ -run TestGoal -race -count=1` passes
- All 5 test cases pass
- Race-clean

---

## Wave 3: Documentation + ECOSYSTEM.md sync

### Task 3.1: Update AGENTS.md

**File:** `AGENTS.md` (in SIN-Code)

- Add `sin_run_loop` and `sin_goal_*` to the MCP tool list in §10
- Update tool count (44+ → 49+)
- Add note: "sin_run_loop is the synchronous delegation path; sin_goal_* is the asynchronous autonomy path"
- Update §7 config contract if new config keys are needed (none expected — all args are per-call)

### Task 3.2: Update ECOSYSTEM.md

**File:** `ECOSYSTEM.md`

- Update the "opencode integration" row to note that opencode can now delegate full verified loops via `sin_run_loop` and enqueue autonomous goals via `sin_goal_add`
- Update MCP tool count

### Task 3.3: Update CHANGELOG.md

**File:** `CHANGELOG.md`

Add entry under the next version:
```
## [Unreleased]
### Added
- `sin_run_loop` MCP tool: synchronous full-agent-loop delegation (PLAN→ACT→VERIFY→DONE) via MCP. Any MCP client (opencode, Claude Code, Codex) can now delegate a complete verified task in one call.
- `sin_goal_add`, `sin_goal_list`, `sin_goal_status`, `sin_goal_complete` MCP tools: asynchronous goal queue management via MCP. Enqueue goals for the daemon, poll status, mark complete.
```

### Task 3.4: CoDocs companion

**Files:** `cmd/sin-code/internal/serve_loop_handler.doc.md`, `cmd/sin-code/internal/serve_goal_handler.doc.md`

Each with `Purpose` + `Docs:` header per the CoDocs standard.

---

## Wave 4: Verification

### Task 4.1: Build + vet + test

```bash
cd /Users/jeremy/dev/SIN-Code
go build ./cmd/sin-code/...
go vet ./cmd/sin-code/...
go test ./cmd/sin-code/internal/ -run "TestRunLoop|TestGoal" -race -count=1
go test ./cmd/sin-code/... -race -count=1
```

### Task 4.2: Manual E2E test via opencode

1. Start `sin-code serve` (stdio mode)
2. From opencode, call `sin_run_loop` with prompt `"list all files in the current directory"`
3. Verify: returns JSON with `verified: true`, `turns: 1-3`
4. From opencode, call `sin_goal_add` with prompt `"add a README.md file"`
5. Verify: returns `goal_id` ≥ 1
6. Call `sin_goal_status` with the goal ID
7. Verify: returns goal details

### Task 4.3: Verify opencode can actually call the tools

- Check `~/.config/opencode/opencode.jsonc` — `sin-code` MCP server is already registered and enabled
- After `sin-code serve` picks up the new tools, opencode's MCP client will auto-discover them
- No opencode config changes needed — the tools are discovered via the MCP manifest

---

## Dependencies

- Wave 1 and Wave 2 are independent — can be implemented in parallel
- Wave 3 depends on both Wave 1 and Wave 2
- Wave 4 depends on Wave 3

## Risk Summary

| Risk | Mitigation |
|---|---|
| `sin_run_loop` blocks the MCP serve goroutine for minutes | Each MCP tool call runs in its own goroutine; the serve server remains responsive. The loop's `MaxTurns` bounds execution time. |
| `sin_run_loop` needs LLM credentials that the serve process may not have | The serve process inherits env vars from the operator. If `OPENAI_API_KEY` (or equivalent) is set, the loop works. If not, the loop fails fast with a clear error. |
| `sin_goal_add` enqueues but daemon isn't running | This is by design — the goal sits in `pending` until the daemon starts. `sin_goal_status` shows `pending`. Operator starts `sin-code daemon` separately. |
| Permission engine blocks `sin_run_loop` in headless mode | By design (M4). Operator passes `yolo: true` or configures `sin_run_loop` as `allow` in their config. |
| Loop tries to use tools that aren't available in serve mode | The loop's tool factory is wired by `loopbuilder.Build()` which includes all standard tools (`sin_read`, `sin_write`, `sin_edit`, `sin_bash`, etc.). MCP client tools are optional via `SkipMCP` flag. |
