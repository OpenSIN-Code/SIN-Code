# dataset/runner.go

## What

Execute one `*Dataset` against the agent loop and capture per-case
outcomes in JSON-friendly `RunResult` rows.

## API surface

| Symbol | Purpose |
|--------|---------|
| `NewRunner(cfg, loop, store)` | validator + defaults: 1× concurrency, 5 min/case timeout |
| `(*Runner).RunDataset(ctx, ds)` | serially evaluate every TestCase |
| `(*Runner).RunCase(ctx, tc)` | single TestCase, used by CLI's progress stream |

## Divergence from the issue body

The issue's reference code defines
`agentloop.Loop.Run(ctx, sessID, prompt, RunOptions{...})`. The
real agentloop API (cmd/sin-code/internal/agentloop/loop.go:151)
is `(*Loop).Run(ctx, *session.Session, prompt) (*Result, error)`
where `Result` carries only `SessionID / Summary / Verified / Turns`.

Adaptation:
- We pass `*session.Session` rather than `sessID + RunOptions`.
- Headless / VerifyCommand / MaxTurns are NOT production knobs from
  the Loop signature — they are dataset fields (`Constraints.MaxTurns`,
  `VerifyCmd`) which we interpret here. The Runner enforces the
  constraint by checking against `Result.Turns`, not by injecting it
  into the loop.
- The referenced `agentloop.RunOptions.ResponseFormat` /
  `MaxTokens` fields do not exist; we read `Constraints.MaxTokens`
  from the dataset but do not pass it down (the LLM layer is
  upstream of this package). Keeping the field on the schema is
  forward-compatible.
- "ToolsUsed" / "FinalOutput" come from `Result.Summary` and
  `ToolsUsed` is populated by callers that track it (today none of
  the existing loops do). The runner does not silently fabricate.

## Rules evaluated

| Source | Rule | Behaviour |
|--------|------|-----------|
| `Constraints.MaxTurns` | `>0` and `res.Turns > N` | mark Success=false |
| `Constraints.RequireVerify` | `true` and not `Verified` | mark Success=false |
| `Constraints.MustUseTools` | any element missing from `ToolsUsed` | mark Success=false |
| `Constraints.ForbiddenTools` | any element present in `ToolsUsed` | mark Success=false |
| `Expected.OutputContains` | substring not found (case-insensitive) | mark Success=false |
| `Expected.OutputAvoids` | substring found (case-insensitive) | mark Success=false |

`MinQuality` is owned by the LLM-as-a-Judge (eval/judge.go).
The runner leaves it 0 so the CLI can layer judge results on
top of structural pass/fail without duplicating logic.
