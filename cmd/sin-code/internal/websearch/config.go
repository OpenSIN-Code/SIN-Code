// SPDX-License-Identifier: MIT
// Purpose: env-driven Config loader for the websearch engine. Pulls
// the four API-key env vars into a Config so callers do not have to
// touch os.Getenv directly. Missing keys leave the corresponding
// providers inactive (Search returns [] for them).
package websearch

import (
	"os"
	"time"
)

// LoadConfig reads the WEBSEARCH_* env vars and returns a fully
// populated Config. All fields are zero-on-missing; callers that want
// to inject a custom HTTPDoer should set cfg.HTTPClient after this
// returns.
func LoadConfig() Config {
	return Config{
		SerpAPIKey: os.Getenv("WEBSEARCH_SERPAPI_KEY"),
		BraveKey:   os.Getenv("WEBSEARCH_BRAVE_KEY"),
		BingKey:    os.Getenv("WEBSEARCH_BING_KEY"),
		TavilyKey:  os.Getenv("WEBSEARCH_TAVILY_KEY"),
		UserAgent:  defaultUserAgent(),
		Timeout:    defaultTimeout(),
	}
}

func defaultUserAgent() string {
	if v := os.Getenv("WEBSEARCH_USER_AGENT"); v != "" {
		return v
	}
	return "sin-code/1.0 (+native websearch)"
}

func defaultTimeout() time.Duration {
	return 15 * time.Second
}
