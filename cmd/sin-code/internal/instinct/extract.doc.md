# instinct/extract.go — pattern mining

Two extractors live here:

## HeuristicExtractor

- Groups observations by `(domain, normalized_action)`
- Emits a candidate only when count ≥ `MinRepeats` (default 2)
- Free of any LLM dependency; safe to call inline

Why this exists: a session can have hundreds of observations; we never
want to feed all of them to a model. The heuristic pass prunes
single-occurrence noise (which is most of it) before the LLM sees
anything.

## LLMExtractor

- Uses a cheap background model (Haiku-class) to mine richer patterns
- Falls back to HeuristicExtractor on any error so the learning loop
  never breaks a session

Wiring: implement `instinct.Completer` against your model client and
pass it to `LLMExtractor{Model: ...}`. See `internal/adapters/completer.go`
for the reference adapter.

## Domain classification

The extractors don't classify domains themselves — they consume
`Observation.Domain` which the hook dispatcher sets via
`Classify(tool, meta)` in `classify.go`. The classifier is the quality
lever: better domains → better instincts.

## Related files

- `classify.go` — domain inference
- `manager.go` — `Observer.Flush` calls `Extractor.Extract`
- `inject.go` — uses `Manager.Active()` to build the prompt block
