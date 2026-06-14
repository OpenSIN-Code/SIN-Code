// internal/agentloop/headroom_hook.go
package agentloop

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
)

// HeadroomHook is a pre-request hook that compresses outgoing LLM messages.
type HeadroomHook struct {
	compressor *headroom.Compressor
	enabled    bool
}

// NewHeadroomHook creates a hook from configuration.
func NewHeadroomHook(cfg headroom.Config) (*HeadroomHook, error) {
	if !cfg.Enabled {
		return &HeadroomHook{enabled: false}, nil
	}
	comp := headroom.NewCompressor(cfg)
	if err := comp.Start(context.Background()); err != nil {
		// Don't fail the whole agent, just disable
		return &HeadroomHook{enabled: false}, nil
	}
	return &HeadroomHook{
		compressor: comp,
		enabled:    true,
	}, nil
}

// PreRequest compresses the content of the request messages.
// It returns modified messages and any error.
func (h *HeadroomHook) PreRequest(ctx context.Context, messages []map[string]interface{}) ([]map[string]interface{}, error) {
	if !h.enabled {
		return messages, nil
	}

	modified := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		// Deep copy to avoid mutation
		clone := make(map[string]interface{})
		for k, v := range msg {
			clone[k] = v
		}

		// Compress content if it's a string
		if content, ok := msg["content"].(string); ok && content != "" {
			compressed, result, err := h.compressor.CompressContent(ctx, content)
			if err != nil {
				// Log but continue with original
				clone["content"] = content
			} else {
				clone["content"] = compressed
				if result != nil && result.RetrievalKeys != nil {
					// Optionally add retrieval keys as metadata for later
					clone["_headroom_keys"] = result.RetrievalKeys
				}
			}
		}
		modified[i] = clone
	}
	return modified, nil
}

// OnFailure sends the session log to headroom learn.
func (h *HeadroomHook) OnFailure(ctx context.Context, sessionLog string) error {
	if !h.enabled {
		return nil
	}
	return h.compressor.LearnFromFailure(ctx, sessionLog)
}

// Stats returns current compression statistics.
func (h *HeadroomHook) Stats() *headroom.Stats {
	if !h.enabled {
		return &headroom.Stats{}
	}
	return h.compressor.GetStats()
}

// Close cleans up resources.
func (h *HeadroomHook) Close() error {
	if h.compressor != nil {
		return h.compressor.Close()
	}
	return nil
}
