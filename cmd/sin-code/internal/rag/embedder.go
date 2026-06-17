// SPDX-License-Identifier: MIT
// Purpose: Embedder interface + dimensions constants. The interface
// is the abstraction that lets the rest of the package be
// implementation-agnostic: HashEmbedder for tests + default,
// ONNXRuntimeEmbedder for production (see onnx_embedder.go).
package rag

import "context"

// EmbeddingDim is the canonical dimension for all embedders in
// this package. 384 matches the sentence-transformers/all-MiniLM-L6-v2
// model the issue body references. HashEmbedder produces exactly
// this dimension; ONNXRuntimeEmbedder must also produce this
// dimension (asserted in NewONNXRuntimeEmbedder).
const EmbeddingDim = 384

// Embedder turns a text into a fixed-dimension float vector. The
// returned slice MUST have length EmbeddingDim; the retriever
// asserts this and returns an error otherwise.
type Embedder interface {
	// Embed returns the embedding for text. Implementations must
	// be safe for concurrent use across many goroutines (the
	// worker pool in worker.go fans out embedding calls).
	Embed(ctx context.Context, text string) ([]float32, error)

	// Dim returns the embedding dimension. Used by the storage
	// layer to assert shape compatibility.
	Dim() int
}

// CosineSimilarity returns the cosine similarity of two non-zero
// vectors. Returns 0 if either vector is zero-length (the calling
// code treats that as "no signal" and excludes the match). The
// computation is in float64 to avoid float32 precision drift over
// 384-dim vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		da, db := float64(a[i]), float64(b[i])
		dot += da * db
		na += da * da
		nb += db * db
	}
	if na == 0 || nb == 0 {
		return 0
	}
	// Cosine is in [-1, 1] but with L2-normalized vectors (which
	// our HashEmbedder produces) it falls in [0, 1] for non-negative
	// embeddings. We clamp defensively for any embedder that
	// produces negative values.
	v := float32(dot / (sqrt(na) * sqrt(nb)))
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// sqrt is a tiny local replacement for math.Sqrt to keep this file
// dependency-free of the math package (the package is small, the
// call site is hot in cosine-sim top-N, and the savings are
// non-trivial for large indexes). It is a variable so tests can
// temporarily override it to exercise defensive clamp paths.
var sqrt = func(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton-Raphson with 8 iterations — converges to float64
	// precision for any positive input in well under 8 steps.
	z := x
	for i := 0; i < 8; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// Normalize L2-normalizes a vector in place. Returns a new slice
// (never mutates the input — the caller may reuse v).
func Normalize(v []float32) []float32 {
	out := make([]float32, len(v))
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return out
	}
	inv := 1.0 / sqrt(sum)
	for i, x := range v {
		out[i] = float32(float64(x) * inv)
	}
	return out
}
