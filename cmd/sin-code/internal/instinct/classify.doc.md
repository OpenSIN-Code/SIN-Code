# instinct/classify.go — domain inference

A simple heuristic that turns `(tool, meta)` into a domain string. This
is the single biggest lever for instinct quality: better domains →
better groupings in `evolve.go` → more useful clusters.

## Layers (in order)

1. **Command prefixes** — `git commit` → `git`, `go test` → `testing`
2. **Path components** — `_test.go` → `testing`, `.env` → `security`
3. **File extensions** — `.go` → `go`, `.ts` → `typescript`
4. **Tool fallback** — `Edit`/`Write` → `code-style`, `Read` → `navigation`
5. **`general`** — last resort

## Why no model here

Classification must be inline in the hook dispatcher. Calling an LLM
on every tool call would multiply cost by 5-10x. The heuristic covers
~85% of useful cases; the LLM extractor (in `extract.go`) handles the
ambiguous 15%.

## Extending

Add new domains by editing the switch statements. Keep the function
deterministic and < 1µs — it runs in the hot path of `PostToolUse`.
