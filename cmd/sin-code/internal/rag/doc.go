// SPDX-License-Identifier: MIT
// Purpose: RAG (retrieval-augmented generation) over the instinct
// store (issue #160). Selects the top-N most relevant active
// instincts per turn via cosine similarity, replacing the
// "all active" prompt block with a focused top-5 block.
//
// The package reuses `internal/memory/Store` (bbolt) for the
// vector index — see index.go. Embeddings come from an Embedder
// interface; the default is HashEmbedder (deterministic, 384-dim,
// no I/O). An ONNXRuntimeEmbedder is sketched in onnx_embedder.go
// for the operator who wants to swap in a real model.
//
// Mandates (issue #160):
//   - M2: Embedder interface is the abstraction. Hash-embedder
//     ships in the binary (no Sideloading). ONNX-embedder is a
//     build-tag-gated file the operator enables after the model
//     is downloaded and the CGO-free runtime is verified.
//   - M5: this package lives under cmd/sin-code/internal/rag/.
//   - M7: embeddings are produced by a bounded-concurrency worker
//     pool (see worker.go) — the agent loop never blocks.
//
// Docs: rag.doc.md
package rag
