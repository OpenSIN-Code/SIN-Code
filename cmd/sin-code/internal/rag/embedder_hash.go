// SPDX-License-Identifier: MIT
// Purpose: HashEmbedder — deterministic, dependency-free embedder
// for tests + default. Produces EmbeddingDim (384) float32 values
// from a SHA-256-seeded RNG, so the same text always maps to the
// same vector. This is NOT semantically meaningful (real embeddings
// capture meaning; hash embeddings capture only "this exact byte
// sequence"), but it is:
//
//   - Deterministic (test-friendly, no model download)
//   - Side-effect free (M2: no ONNX Sideloading required)
//   - Fast (microseconds per call)
//   - Drop-in compatible with the Embedder interface
//
// Production deployments should swap in ONNXRuntimeEmbedder
// (see onnx_embedder.go). The swap is a one-line change in
// the worker pool constructor.
package rag

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
)

// HashEmbedder is the default Embedder. The seed is taken from
// SHA-256(text), so the same text produces the same vector
// across processes and platforms.
type HashEmbedder struct{}

// NewHashEmbedder returns a HashEmbedder. There is no configuration
// because the embedder is content-addressed.
func NewHashEmbedder() *HashEmbedder { return &HashEmbedder{} }

// Embed implements Embedder.
func (h *HashEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	if text == "" {
		// Empty text returns a zero vector. The retriever excludes
		// zero vectors (cosine with a zero is 0), so empty prompts
		// fall through to "no relevant instincts".
		return make([]float32, EmbeddingDim), nil
	}
	// HashExpand produces EmbeddingDim floats from SHA-256(text).
	// We do it in 4 batches (96 bytes each → 24 floats per batch
	// if we read 4 bytes per float, or 32 batches of 12 floats if
	// we read 3 bytes per float). The simpler scheme: 4-byte chunks
	// (uint32), big-endian, mapped to [0, 1] then centered to
	// [-0.5, 0.5]. 384 = 4 * 96, so we need 4 * 96 = 384 uint32s,
	// which is 4 * 384 = 1536 bytes of entropy. SHA-256 gives us
	// only 32 bytes; we expand via SHA-256 chaining.
	out := make([]float32, EmbeddingDim)
	var seed [32]byte
	copy(seed[:], sha256Sum(text))
	for i := 0; i < EmbeddingDim; i++ {
		if i%32 == 0 {
			// Re-seed: SHA-256 of the previous seed + i.
			next := sha256.New()
			next.Write(seed[:])
			var idx [4]byte
			binary.BigEndian.PutUint32(idx[:], uint32(i))
			next.Write(idx[:])
			seed = sha256.Sum256(next.Sum(nil))
		}
		// Take 4 bytes of the seed and turn into a float in [-0.5, 0.5].
		off := i % 32
		// Wrap if we run off the end of seed.
		offA := off
		offB := (off + 1) % 32
		offC := (off + 2) % 32
		offD := (off + 3) % 32
		u := uint32(seed[offA])<<24 | uint32(seed[offB])<<16 |
			uint32(seed[offC])<<8 | uint32(seed[offD])
		// Map to [-0.5, 0.5].
		out[i] = float32(u)/float32(1<<32) - 0.5
	}
	return Normalize(out), nil
}

// Dim implements Embedder.
func (h *HashEmbedder) Dim() int { return EmbeddingDim }

// sha256Sum is a tiny helper to keep Embed readable. Returns the
// 32-byte SHA-256 hash of text.
func sha256Sum(text string) []byte {
	h := sha256.Sum256([]byte(text))
	return h[:]
}
