// SPDX-License-Identifier: MIT
// Purpose: config loader for the native web-search engine (issue #381).
// Reads API keys from the environment and builds a Config that the Engine
// uses to decide which providers are active. Each provider is gated by its
// own env key so the engine degrades gracefully when keys are absent.
package websearch

import (
	"os"
	"strings"
	"time"
)

// Config holds resolved settings for the web-search Engine.
type Config struct {
	// EnabledProviders is the ordered list of provider names that have a
	// usable API key (or are keyless, e.g. DuckDuckGo).
	EnabledProviders []string
	// SerpAPIKey, BraveKey, TavilyKey are API keys read from the env.
	SerpAPIKey string
	BraveKey   string
	TavilyKey  string
	// DefaultTimeout is the per-provider HTTP timeout.
	DefaultTimeout time.Duration
	// MaxResults caps the number of results returned per query.
	MaxResults int
}

// Env key names for each provider.
const (
	EnvSerpAPIKey = "WEBSEARCH_SERPAPI_KEY"
	EnvBraveKey   = "WEBSEARCH_BRAVE_KEY"
	EnvTavilyKey  = "WEBSEARCH_TAVILY_KEY"
)

// LoadConfig reads the environment and returns a Config with only the
// providers that have credentials (DuckDuckGo is always available because
// it is keyless). Missing keys are silently skipped — the engine works
// with whatever providers are configured.
func LoadConfig() Config {
	cfg := Config{
		DefaultTimeout: 15 * time.Second,
		MaxResults:     10,
	}
	// DuckDuckGo is keyless — always enabled.
	cfg.EnabledProviders = append(cfg.EnabledProviders, "duckduckgo")

	cfg.SerpAPIKey = strings.TrimSpace(os.Getenv(EnvSerpAPIKey))
	if cfg.SerpAPIKey != "" {
		cfg.EnabledProviders = append(cfg.EnabledProviders, "serpapi")
	}
	cfg.BraveKey = strings.TrimSpace(os.Getenv(EnvBraveKey))
	if cfg.BraveKey != "" {
		cfg.EnabledProviders = append(cfg.EnabledProviders, "brave")
	}
	cfg.TavilyKey = strings.TrimSpace(os.Getenv(EnvTavilyKey))
	if cfg.TavilyKey != "" {
		cfg.EnabledProviders = append(cfg.EnabledProviders, "tavily")
	}
	return cfg
}
