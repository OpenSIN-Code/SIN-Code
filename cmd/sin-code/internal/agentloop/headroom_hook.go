// SPDX-License-Identifier: MIT
// Purpose: Headroom compression adapter for the agent loop (issue #118).
// Bridges the headroom.Compressor to the loop's CompressMessages hook so
// outgoing model requests are compressed transparently. Compression failures
// never break a run — the original history is always preserved.
package agentloop

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
)

// HeadroomHook wraps a headroom.Compressor and adapts it to the agent loop's
// CompressMessages signature.
type HeadroomHook struct {
	compressor *headroom.Compressor
	enabled    bool
}

// NewHeadroomHook builds a hook from configuration. If headroom is disabled or
// the backend cannot be started, it returns a disabled hook that passes
// messages through unchanged (never an error).
func NewHeadroomHook(cfg headroom.Config) *HeadroomHook {
	if !cfg.Enabled {
		return &HeadroomHook{enabled: false}
	}
	comp := headroom.NewCompressor(cfg)
	if err := comp.Start(context.Background()); err != nil {
		return &HeadroomHook{enabled: false}
	}
	return &HeadroomHook{compressor: comp, enabled: true}
}

// Enabled reports whether compression is active.
func (h *HeadroomHook) Enabled() bool { return h != nil && h.enabled }

// Compressor exposes the underlying compressor (may be nil when disabled).
func (h *HeadroomHook) Compressor() *headroom.Compressor {
	if h == nil {
		return nil
	}
	return h.compressor
}

// Close releases backend resources held by the compressor.
func (h *HeadroomHook) Close() error {
	if h == nil || h.compressor == nil {
		return nil
	}
	return h.compressor.Close()
}

// CompressMessages compresses the string content of each message. It matches
// the agentloop.Loop.CompressMessages signature. On any per-message failure it
// keeps the original content for that message, so a run never breaks.
func (h *HeadroomHook) CompressMessages(ctx context.Context, msgs []session.Message) ([]session.Message, error) {
	if !h.Enabled() || len(msgs) == 0 {
		return msgs, nil
	}

	out := make([]session.Message, len(msgs))
	copy(out, msgs)
	for i := range out {
		if out[i].Content == "" {
			continue
		}
		compressed, _, err := h.compressor.CompressContent(ctx, out[i].Content)
		if err != nil || compressed == "" {
			continue // preserve original on failure
		}
		out[i].Content = compressed
	}
	return out, nil
}
