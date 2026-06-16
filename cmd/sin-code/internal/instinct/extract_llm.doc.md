# instinct/extract_llm.go — model-backed pattern mining

The heuristic extractor is fast and model-free but shallow. This
extractor adds a cheap background model (Haiku-class) to the loop
to mine richer patterns. It is *opt-in* — you only get charged when
you wire a `Completer` into the Observer.

## Design rules

1. **Never block the session.** On any error (network, parse, model
   refusal) the extractor falls back to `HeuristicExtractor`. The
   agent never waits on instinct mining.
2. **Cap the input.** `MaxObs` (default 40) bounds what we send to
   the model. Older observations are dropped, not truncated, so the
   model sees a coherent recent window.
3. **Tolerate model sloppiness.** `parseCandidatesJSON` strips code
   fences and embedded prose before parsing. Empty `action` is
   silently dropped.
4. **No domain classification in the prompt.** The model is told the
   domain is one of `git|testing|...` but the actual value comes from
   `Classify(tool, meta)` upstream — this prevents the model from
   inventing custom domains that wouldn't cluster well in `evolve.go`.

## Wire-up

```go
obs := instinct.NewObserver(mgr, instinct.LLMExtractor{
    Model:  myBackgroundModelAdapter, // implements Completer
    MaxObs: 40,
})
```

See `internal/adapters/completer.go` for a reference adapter.
