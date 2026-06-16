# instinct/inject.go — close the loop

The single most important file in this package. The model has to see
the result of its own past behavior, otherwise "learning" is just
collection. This is where the block that goes into the system prompt
is built.

## Wire-up

```go
block, _ := mgr.SystemBlockForProject(15)
if block != "" {
    systemPrompt += "\n\n" + block
}
```

## Strength labels

| Confidence | Label | Effect |
|---|---|---|
| 0.60–0.65 | `consider` | soft hint, model may override |
| 0.65–0.80 | `prefer` | should follow unless explicit reason not to |
| 0.80+ | `strongly prefer` | treat as a hard habit |

The thresholds are encoded in the function, not in config, because
they correspond to the activate/prefer/dominate regime transitions
in the confidence math (see `types.go`).

## Cap

`max` (default 15 in agent loop usage) keeps the prompt compact. The
strongest instincts always make the cut; weaker ones get rotated in
as confidence moves.
