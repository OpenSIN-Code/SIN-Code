// SPDX-License-Identifier: MIT
// Purpose: Fireworks AI multi-model pool for SIN Fusion v1 (issue #290).
//
// All tournament participants share the same Fireworks base URL and API
// key (the SINator pool router handles key rotation). Each model becomes
// one ProviderConfig — diversity comes from different model architectures,
// not different providers.
//
// Default lineup (strongest Asian open models on Fireworks, 2026-06):
//
//   1. MiniMax M3          — accounts/fireworks/models/minimax-m3
//   2. Kimi K2.7 Code Fast — accounts/fireworks/routers/kimi-k2p7-code-fast
//   3. Kimi K2.7 Code      — accounts/fireworks/models/kimi-k2p7-code
//   4. DeepSeek V4 Pro     — accounts/fireworks/models/deepseek-v4-pro
//   5. Qwen 3.7 Plus       — accounts/fireworks/models/qwen3p7-plus
//   6. GLM 5.2             — accounts/fireworks/models/glm-5p2
package fusion

import (
	"os"
	"strings"
)

// FireworksModel is one entry in the Fireworks pool lineup.
type FireworksModel struct {
	Name        string  // tournament display name (e.g. "minimax-m3")
	Slug        string  // Fireworks model slug (e.g. "accounts/fireworks/models/minimax-m3")
	InputPer1M  float64 // USD per 1M input tokens
	OutputPer1M float64 // USD per 1M output tokens
	ContextLen  int     // max context window in tokens
	Vision      bool    // supports image input
	Thinking    bool    // supports reasoning/thinking mode
}

// DefaultFireworksLineup is the curated set of strongest Asian open models
// available on Fireworks AI as of 2026-06. All are routed through the
// SINator pool proxy for key rotation. Prices verified from fireworks.ai.
var DefaultFireworksLineup = []FireworksModel{
	{Name: "minimax-m3", Slug: "accounts/fireworks/models/minimax-m3", InputPer1M: 0.30, OutputPer1M: 1.20, ContextLen: 524288, Vision: true, Thinking: true},
	{Name: "kimi-k2p7-code-fast", Slug: "accounts/fireworks/routers/kimi-k2p7-code-fast", InputPer1M: 0.95, OutputPer1M: 4.00, ContextLen: 262144, Vision: true, Thinking: true},
	{Name: "kimi-k2p7-code", Slug: "accounts/fireworks/models/kimi-k2p7-code", InputPer1M: 0.95, OutputPer1M: 4.00, ContextLen: 262144, Vision: true, Thinking: true},
	{Name: "deepseek-v4-pro", Slug: "accounts/fireworks/models/deepseek-v4-pro", InputPer1M: 1.74, OutputPer1M: 3.48, ContextLen: 1048576, Vision: false, Thinking: true},
	{Name: "qwen-3p7-plus", Slug: "accounts/fireworks/models/qwen3p7-plus", InputPer1M: 0.40, OutputPer1M: 1.60, ContextLen: 262144, Vision: true, Thinking: true},
	{Name: "glm-5p2", Slug: "accounts/fireworks/models/glm-5p2", InputPer1M: 1.40, OutputPer1M: 4.40, ContextLen: 1048576, Vision: false, Thinking: true},
}

// LoadFireworksPool builds ProviderConfigs from the Fireworks model lineup.
// All models share the same base URL and API key. The base URL defaults to
// the SINator pool router (key rotation proxy); the API key defaults to
// $FIREWORKS_API_KEY.
//
// If `modelNames` is non-empty, only models whose Name matches are included
// (same filter semantics as LoadProviderPool). An empty list = all models.
func LoadFireworksPool(lineup []FireworksModel, modelNames []string) []ProviderConfig {
	if len(lineup) == 0 {
		lineup = DefaultFireworksLineup
	}

	baseURL := os.Getenv("FIREWORKS_BASE_URL")
	if baseURL == "" {
		baseURL = "https://sinatorpool-router.delqhi.com/inference/v1"
	}
	apiKey := os.Getenv("FIREWORKS_API_KEY")

	nameFilter := make(map[string]bool, len(modelNames))
	for _, n := range modelNames {
		nameFilter[strings.TrimSpace(n)] = true
	}

	var pool []ProviderConfig
	for _, m := range lineup {
		if len(nameFilter) > 0 && !nameFilter[m.Name] {
			continue
		}
		pool = append(pool, ProviderConfig{
			Name:        m.Name,
			Model:       m.Slug,
			BaseURL:     baseURL,
			APIKey:      apiKey,
			InputPer1M:  m.InputPer1M,
			OutputPer1M: m.OutputPer1M,
			MaxTokens:   8192,
			Vision:      m.Vision,
			Thinking:    m.Thinking,
		})
	}
	return pool
}
