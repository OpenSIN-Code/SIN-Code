// internal/headroom/config.go
package headroom

import (
	"os"
	"strings"
	"time"
)

// LoadConfigFromEnv reads headroom configuration from environment variables.
func LoadConfigFromEnv() Config {
	cfg := DefaultConfig()

	if v := os.Getenv(EnvEnabled); v != "" {
		cfg.Enabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv(EnvMode); v != "" {
		switch strings.ToLower(v) {
		case "proxy":
			cfg.Mode = ModeProxy
		case "mcp":
			cfg.Mode = ModeMCP
		case "cli":
			cfg.Mode = ModeCLI
		}
	}
	if v := os.Getenv(EnvProxyURL); v != "" {
		cfg.ProxyURL = v
	}
	if v := os.Getenv(EnvCompressionLevel); v != "" {
		cfg.CompressionLevel = v
	}
	if v := os.Getenv(EnvLearnFromFailures); v != "" {
		cfg.LearnFromFailures = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv(EnvStatsEnabled); v != "" {
		cfg.StatsEnabled = strings.ToLower(v) == "true" || v == "1"
	}
	if v := os.Getenv(EnvTimeout); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Timeout = d
		}
	}
	if v := os.Getenv(EnvCacheEnabled); v != "" {
		cfg.CacheEnabled = strings.ToLower(v) == "true" || v == "1"
	}

	return cfg
}
