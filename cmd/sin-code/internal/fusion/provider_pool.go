// SPDX-License-Identifier: MIT
// Purpose: Provider pool for SIN Fusion v1 (issue #290).
//
// Loads agent profiles from profiles/*.toml and converts them into
// ProviderConfig entries for the tournament. Each profile becomes one
// tournament participant with its own llm.Client, model, and base URL.
package fusion

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

// LoadProviderPool reads all profiles/*.toml from the given profiles
// directory, filters by the `names` list (empty = load all), and returns
// a slice of ProviderConfig suitable for Tournament.Providers.
//
// API keys are resolved from the environment variable named by the
// profile's `provider` field uppercased + "_API_KEY" (e.g. provider
// "fireworks" → FIREWORKS_API_KEY). If the env var is not set, the
// APIKey field is left empty (the provider will fail at call time,
// which the tournament handles gracefully).
func LoadProviderPool(profilesDir string, names []string) ([]ProviderConfig, error) {
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return nil, fmt.Errorf("fusion: read profiles dir %s: %w", profilesDir, err)
	}

	nameFilter := make(map[string]bool, len(names))
	for _, n := range names {
		nameFilter[strings.TrimSpace(n)] = true
	}

	var pool []ProviderConfig
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		path := filepath.Join(profilesDir, entry.Name())
		var cfg orchestrator.AgentConfig
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("fusion: read profile %s: %w", path, err)
		}
		if _, err := toml.Decode(string(data), &cfg); err != nil {
			return nil, fmt.Errorf("fusion: decode profile %s: %w", path, err)
		}
		if cfg.Name == "" {
			continue
		}
		if len(nameFilter) > 0 && !nameFilter[cfg.Name] {
			continue
		}

		apiKey := resolveAPIKey(cfg.Provider)

		pool = append(pool, ProviderConfig{
			Name:        cfg.Name,
			Model:       cfg.Model,
			BaseURL:     cfg.BaseURL,
			APIKey:      apiKey,
			MaxTokens:   cfg.MaxTokens,
			InputPer1M:  estimateInputPrice(cfg.Provider, cfg.Model),
			OutputPer1M: estimateOutputPrice(cfg.Provider, cfg.Model),
		})
	}

	return pool, nil
}

// resolveAPIKey looks up the API key from the environment. The convention
// is: provider name uppercased + "_API_KEY". Returns empty string if not
// found — the tournament handles this gracefully (the provider will fail
// at call time and become a loser or be cancelled).
func resolveAPIKey(provider string) string {
	if provider == "" {
		return ""
	}
	return os.Getenv(strings.ToUpper(provider) + "_API_KEY")
}

// estimateInputPrice returns a rough USD-per-1M-input-tokens estimate.
func estimateInputPrice(provider, model string) float64 {
	switch strings.ToLower(provider) {
	case "fireworks":
		return 1.0
	case "qwen-relay":
		return 0.0
	default:
		return 2.0
	}
}

// estimateOutputPrice returns a rough USD-per-1M-output-tokens estimate.
func estimateOutputPrice(provider, model string) float64 {
	switch strings.ToLower(provider) {
	case "fireworks":
		return 3.0
	case "qwen-relay":
		return 0.0
	default:
		return 5.0
	}
}
