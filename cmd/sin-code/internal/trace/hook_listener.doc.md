# trace/hook_listener.go

## What

Maps the **actual** `cmd/sin-code/internal/hooks.Engine.Fire` lifecycle
events to OpenTelemetry spans. This package is the only place where
span names / attributes diverge from the issue body — divider section
below states the divergence and why.

## API surface

| Symbol | Purpose |
|--------|---------|
| `NewHookListener()` | construct; uses the global OTel tracer (from `InitProvider`) |
| `WrapEngine(*hooks.Engine) *EngineWrapper` | drop-in decorator: wrapped `Fire` emits one span per call, then forwards to original |
| `RecordHook(ctx, Payload) (ctx, Span)` | standalone entry-point for code that already has a `hooks.Payload` |
| `EngineWrapper.Fire(ctx, Payload) Result` | identical signature to `hooks.Engine.Fire` — replace field, no other change |

## Divergence from the issue body

The issue's reference code refers to **declarative manager events**:
`EventSessionStart`, `EventPlanStart`, `EventPlanComplete`,
`EventActToolCall`, `EventActToolResult`, `EventVerifyStart`,
`EventVerifyResult`, `EventLessonsQuery`, `EventLessonsApplied`,
`EventLessonsRecorded`, `EventPermissionCheck`,
`EventPermissionDecision`, `EventError`. Those identifiers do not
exist in the repository. The actual hook API
(`cmd/sin-code/internal/hooks/hooks.go:28–74`) declares 24
constants: `SessionStart`, `SessionResume`, `SessionEnd`, `TurnStart`,
`TurnEnd`, `ToolPre`, `ToolPost`, `ToolDenied`, `ToolError`,
`PermissionAsk`, `VerifyPre`, `VerifyPass`, `VerifyFail`,
`AgentSpawn`, `AgentComplete`, `CriticReject`, `AdversaryFinding`,
`GovernorBlock`, `MemoryWrite`, `MemoryCompact`, `CommitPre`,
`CommitPost`, `PushPre`, `TaskComplete`, `TaskAbort`, `CompactionPre`,
`GoalEnqueued`, `GoalStarted`, `GoalVerified`, `GoalExhausted`,
`TriggerFired`, `SkillInstalled`, `SkillFailed` — all string
constants firing through a single `Engine.Fire(ctx, Payload)` entry.

Listener rewrites them into OTel span names by upper-casing each
dot-segment — `session.start` → `SinSessionStart`. Dashboards keyed
off the real event names keep working; the prefix `Sin` avoids
collision with future OTel semconv span names.

## Why decorator wrap (not Manager.On)

The real engine doesn't expose a Subscribe API. `Engine.Fire` is the
only call site where the agent loop actually fires events
(`agentloop/loop.go:88–98, 102, 117, 145, 215, 249, 267`). Wrapping
`Fire` intercepts every event the loop emits, without needing to
patch call sites or maintain a parallel subscription registry.

## Result → span status

| Event | span status |
|-------|-------------|
| `verify.fail`, `tool.error`, `task.abort`, `tool.denied`, `governor.block`, `critic.reject` | `Error` (the `error` or `reason` data field becomes the status message) |
| `verify.pass`, `task.complete`, `goal.verified` | `Ok` (status description = event name) |
| everything else | `Unset` (no explicit status) |
