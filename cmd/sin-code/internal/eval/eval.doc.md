# eval/judge.go + metrics.go

## What

LLM-as-a-Judge (`judge.go`) and suite metrics (`metrics.go`) for the
Eval system (issue #75).

```
   prompt + trajectory ─► Judge.Evaluate(ctx, traj)
                                │
                                ▼ internal/llm.Client.Chat
                          LLM (gpt-4o, Claude, Llama…)
                                │ returns JudgeResult JSON
                                ▼
   dataset.RunResult ◄──────── RunResult.JudgeScore
   ──────────────── aggregated to Summary by Summarise()
```

## API surface

| Symbol | Purpose |
|--------|---------|
| `NewJudge(cfg, client)` | strict validator (`Model` + `MinPassScore`) |
| `(*Judge).Evaluate(ctx, traj)` | one trajectory → one **JudgeResult** |
| `(*Judge).EvaluateBatch(ctx, trajs)` | convenience loop, errors short-circuit |
| `Summarise(rs, min)` | flat aggregates (pass_rate, mean_judge, etc.) |
| `NewReport(...)` | full JSON envelope for `eval run --json` |
| `WriteJSON(w, report)` | stable-indent JSON encoder (so jq stays stable) |
| `PassRateFloor(s)` | returns \*BelowMinRate when `pass_rate < min` |
| `FormatHuman(s)` | CLI pretty-print line-by-line |

## Divergence from the issue body

The issue reference code calls
`client.Chat(ctx, Request{ResponseFormat: llm.ResponseFormatJSON, ...})`
and reads `response.Content` (a flat string). The actual
`internal/llm.ChatRequest` struct has:

- `Model`, `Messages`, `MaxTokens`, `Temperature`, `Stream` — present
- `ResponseFormat` — **not present**

Adaptation:
- We drop the `ResponseFormat` field. The judge relies on the LLM
  returning JSON in prose (model behaviour, not API contract).
- We strip ` ```json ... ``` ` fences around the response before
  parsing so a slightly misbehaving model still works.
- We read `resp.Choices[0].Message.Content` (the real field name),
  not the `response.Content` shortcut in the reference code.
- We always rewrite `Pass` from `Score` and `MinPassScore`
  locally — letting the LLM decide pass/fail invites drift.

## Metric contract

The CI step consumes `.summary.pass_rate` and `.summary.min_required`:

```bash
jq -e '.summary.pass_rate >= .summary.min_required' < report.json
```

That is the only field-level contract this package guarantees.
