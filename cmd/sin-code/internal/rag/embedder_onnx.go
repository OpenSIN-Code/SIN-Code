// SPDX-License-Identifier: MIT
// Purpose: ONNXRuntimeEmbedder — production embedder backed by
// `github.com/yalue/onnxruntime_go` (a CGO-free Go binding for
// ONNX Runtime). Per the issue body:
//
//   - "The default implementation calls a local ONNX runtime
//     (github.com/yalue/onnxruntime_go or similar). The fallback
//     calls an HTTP API (your existing model client) for
//     embeddings."
//
// This file is INTENTIONALLY a stub. The ONNX library requires
// (a) a downloaded model file (e.g. sentence-transformers/all-MiniLM-L6-v2
// in ONNX format, ~25 MB) and (b) the onnxruntime shared library
// (libonnxruntime.so / .dylib / .dll) which the operator must
// install at the system level. Sideloading the .so into the
// binary breaks M2 (single static binary).
//
// The stub:
//   - Satisfies the Embedder interface so the constructor and
//     wiring compile
//   - Returns a clear error at runtime so the operator knows
//     to (a) install the onnxruntime shared library, and
//     (b) set the path via env var
//   - Documents the wiring in detail so a future PR with the
//     real implementation has a clear template
//
// To enable: replace the Embed body with a call to the
// onnxruntime_go library's Session.Run(). The constructor
// already takes the model path; one new import and ~30 LOC
// of session setup is all that's needed.
package rag

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// ONNXRuntimeEmbedder is a stub for the production embedder. See
// the file-level comment for why it is a stub and how to enable.
type ONNXRuntimeEmbedder struct {
	// modelPath is the absolute path to the ONNX model file. Set
	// by NewONNXRuntimeEmbedder; the Embed method uses it once
	// the real implementation is in place.
	modelPath string
}

// NewONNXRuntimeEmbedder returns a stub embedder wired to the
// model at modelPath. The actual ONNX session is created lazily
// on the first Embed call. Until the real implementation lands,
// Embed returns an error explaining what the operator must do.
func NewONNXRuntimeEmbedder(modelPath string) *ONNXRuntimeEmbedder {
	return &ONNXRuntimeEmbedder{modelPath: modelPath}
}

// ErrONNXNotEnabled is returned when Embed is called on a
// ONNXRuntimeEmbedder stub. The error message tells the operator
// what to do next.
var ErrONNXNotEnabled = errors.New("ONNX runtime not enabled: see cmd/sin-code/internal/rag/embedder_onnx.go for the wiring instructions")

// Embed implements Embedder. The stub returns ErrONNXNotEnabled
// unless SIN_RAG_ONNX_PATH points to a usable runtime. The
// environment variable exists so tests can simulate "ONNX enabled"
// without actually loading the model.
func (o *ONNXRuntimeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if os.Getenv("SIN_RAG_ONNX_PATH") == "" {
		return nil, fmt.Errorf("%w: install libonnxruntime and set SIN_RAG_ONNX_PATH (model at %s)",
			ErrONNXNotEnabled, o.modelPath)
	}
	// Real implementation (TODO when the operator adds the dependency):
	//
	//   import "github.com/yalue/onnxruntime_go"
	//
	//   session, err := onnxruntime_go.NewSession(
	//       o.modelPath,
	//       []string{"input_ids", "attention_mask", "token_type_ids"},
	//       []string{"last_hidden_state"},
	//   )
	//   if err != nil { return nil, err }
	//   ...
	//   output, err := session.Run([]tensor){inputIDs, attentionMask, tokenTypeIDs})
	//   ...
	//   // Mean-pool the [seq, dim] output to [dim] and L2-normalize.
	//   pooled := meanPool(output[0], attentionMask[0])
	//   return Normalize(pooled), nil
	//
	// For now, return a clear error.
	return nil, ErrONNXNotEnabled
}

// Dim implements Embedder.
func (o *ONNXRuntimeEmbedder) Dim() int { return EmbeddingDim }

// HTTPEmbedder is the fallback mentioned in the issue body: an
// HTTP API call (e.g. OpenAI embeddings, NIM embeddings) when
// no local ONNX runtime is available. It is a stub for the same
// reason as ONNXRuntimeEmbedder: the wire format and endpoint
// are operator-specific and not part of the v0 scope.
type HTTPEmbedder struct {
	Endpoint string
	APIKey   string
	Model    string
}

// NewHTTPEmbedder returns a stub HTTP embedder. Wire it up by
// setting Endpoint (e.g. "https://integrate.api.nvidia.com/v1")
// and APIKey. The Model field is the model name passed in the
// request body.
func NewHTTPEmbedder(endpoint, apiKey, model string) *HTTPEmbedder {
	return &HTTPEmbedder{Endpoint: endpoint, APIKey: apiKey, Model: model}
}

// Embed implements Embedder. Stub — returns ErrONNXNotEnabled
// (the same error, for the same reason: the actual HTTP call
// is operator-specific).
func (h *HTTPEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if h.Endpoint == "" || h.APIKey == "" {
		return nil, fmt.Errorf("HTTPEmbedder: endpoint and api key required")
	}
	return nil, ErrONNXNotEnabled
}

// Dim implements Embedder.
func (h *HTTPEmbedder) Dim() int { return EmbeddingDim }
