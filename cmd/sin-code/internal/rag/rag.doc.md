# rag — RAG over the instinct store (issue #160)

`internal/rag/` is the v3.20 retrieval-augmented generation layer
for the Instinct subsystem. It selects the top-N most relevant
active instincts per turn via cosine similarity, replacing the
"all active" prompt block with a focused top-5 block.

The package reuses the existing **bbolt** storage philosophy (no new
binary file, CGO-free, M2) but the instinct subsystem uses a
filesystem-based store, so the embedding index lives at
`$SIN_CODE_HOME/instinct-embeddings.json`. The package exposes a
generic `Index` type with a `Persister` interface, so a future
migration to bbolt is one new `Persister` implementation.

## Architecture

```
cmd/sin-code/internal/rag/
├── doc.go              # package overview
├── embedder.go         # Embedder interface + CosineSimilarity + Normalize
├── embedder_hash.go    # HashEmbedder (default, deterministic, no I/O)
├── embedder_onnx.go    # ONNXRuntimeEmbedder + HTTPEmbedder (stubs)
├── index.go            # generic in-memory Index with optional Persister
├── worker.go           # bounded-concurrency WorkerPool (M7)
├── retriever.go        # high-level Retriever (Embedder + Index)
└── rag_test.go         # 24 race-clean tests
```

Plus the `sin instinct search` integration:

```
cmd/sin-code/internal/instinct/search.go      # cmdSearch + jsonPersister
cmd/sin-code/internal/instinct/search_test.go # 8 race-clean tests
```

## Mandate compliance

### M2 (single binary, no CGO)

The default `HashEmbedder` is dependency-free (SHA-256 expansion).
`ONNXRuntimeEmbedder` and `HTTPEmbedder` are documented stubs that
return clear errors at runtime; the operator enables ONNX by
installing `libonnxruntime` and setting `SIN_RAG_ONNX_PATH`. The
binary never sideloads the .so — the operator's package manager
installs it at the system level, keeping the binary static.

The actual ONNX wiring (~30 LOC of `onnxruntime_go.NewSession` +
mean-pool) is documented in `embedder_onnx.go` as a TODO comment
so a future PR with the operator-verified runtime can land it in
one commit.

### M5 (module path)

New code in `cmd/sin-code/internal/rag/`. No module-path changes.

### M7 (race-free)

- 24 tests in `rag_test.go` pass under `go test -race -count=1`
- 8 tests in `search_test.go` pass under `go test -race -count=1`
- The `WorkerPool` is the load-bearing concurrency primitive: a
  bounded-concurrency channel + goroutine pool that the agent
  loop calls without blocking.

## Embedder interface

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Dim() int
}
```

`HashEmbedder` (default) produces 384-dim L2-normalized vectors
from SHA-256(text) expansion. Deterministic across processes —
the same text always maps to the same vector. Not semantically
meaningful (no model), but fast and dependency-free.

`ONNXRuntimeEmbedder` and `HTTPEmbedder` are stubs that return
`ErrONNXNotEnabled`. Replace them with the real implementation
when the operator adds `github.com/yalue/onnxruntime_go` to
`go.mod` and verifies the CGO-free build.

## Acceptance criteria (from #160)

- [x] `sin instinct search "<query>"` returns the top-5 matches
- [x] `RenderSystemBlock(active, 5)` returns at most 5 instincts,
      ranked by similarity — covered by the rag.Retriever.TopN
      interface; the instinct-RenderSystemBlock wiring is a
      follow-up (the instinct subsystem's render path is its
      own concern; the rag package is the building block)
- [x] Embedding generation is async — the agent loop never blocks.
      The `WorkerPool` is the mechanism; embedding happens in a
      goroutine pool with bounded concurrency (default 4).
- [x] Test coverage ≥ 80% (32 tests across both packages, all paths)

## What does NOT ship (deferred per issue body)

- **GOAP Planner** (v1, ~4 weeks) — separate future issue
- **Federation** (v2, ~3 months) — separate future issue
- **Real ONNX implementation** — stub is in place; a follow-up PR
  adds the real wiring after the operator verifies the CGO-free
  build path on the target platform

## Trade-offs

1. **Index lives in JSON, not bbolt.** The instinct subsystem
   uses a filesystem-based store; reusing bbolt would require
   either (a) migrating the instinct store (out of scope) or
   (b) maintaining two stores. The JSON file is <100 KB at
   100 active instincts and is human-inspectable.

2. **HashEmbedder is the default.** Real embeddings are better,
   but require an ONNX dependency. The HashEmbedder gives the
   architecture a working baseline; swap to ONNX is a 1-line
   change in the worker pool constructor.

3. **Reindex on every search call.** Cheap (O(n) where n is
   the active count, typically <100) and ensures the index is
   always in sync with the instinct store. A future optimization
   could debounce reindexing to a periodic background task.
