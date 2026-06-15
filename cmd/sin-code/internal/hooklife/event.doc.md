# hooklife/event.go — types

Core types every other file in this package builds on.

## `Phase`

| Phase | When | May Block? |
|---|---|---|
| `PreToolUse` | before a tool runs | yes |
| `PostToolUse` | after a tool runs | no (Warn only) |
| `Stop` | turn finished | no |
| `SessionStart` | session begins | no |
| `SessionEnd` | session ends | no |
| `PreCompact` | before context compaction | no |
| `UserPrompt` | a user prompt was submitted | yes |

## `Verdict`

| Verdict | Effect on runner |
|---|---|
| `Allow` | proceed silently |
| `Warn` | proceed + surface `Message` to the user/agent |
| `Block` | veto the action (PreToolUse only) |

`Block` outside of `PreToolUse` is recorded as a warning in the
aggregate — the runner cannot actually stop a non-tool lifecycle point
that has already started.

## Related files

- `registry.go` — stores hooks by phase
- `runner.go` — applies the verdict semantics
- `builtin.go` — concrete hooks
