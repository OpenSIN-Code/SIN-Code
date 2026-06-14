// internal/headroom/types.go
package headroom

import (
	"time"
)

// Config holds Headroom integration configuration
type Config struct {
	Enabled           bool          `json:"enabled"`             // HEADROOM_ENABLED
	Mode              Mode          `json:"mode"`                // proxy, mcp, cli
	ProxyURL          string        `json:"proxy_url"`           // HEADROOM_PROXY_URL (default: http://localhost:8787/v1)
	CompressionLevel  string        `json:"compression_level"`   // light, normal, aggressive
	LearnFromFailures bool          `json:"learn_from_failures"` // HEADROOM_LEARN
	StatsEnabled      bool          `json:"stats_enabled"`       // HEADROOM_STATS
	Timeout           time.Duration `json:"timeout"`             // HEADROOM_TIMEOUT (default: 30s)
	CacheEnabled      bool          `json:"cache_enabled"`       // HEADROOM_CACHE
}

// Mode defines how SIN-Code talks to Headroom
type Mode string

const (
	ModeProxy Mode = "proxy" // HTTP proxy mode (zero code change)
	ModeMCP   Mode = "mcp"   // Native MCP tools
	ModeCLI   Mode = "cli"   // Direct CLI invocation (headroom compress)
)

// CompressionResult holds the output of a compression operation
type CompressionResult struct {
	OriginalContent   string   `json:"original_content"`
	CompressedContent string   `json:"compressed_content"`
	OriginalTokens    int      `json:"original_tokens,omitempty"`
	CompressedTokens  int      `json:"compressed_tokens,omitempty"`
	SavingsPercent    float64  `json:"savings_percent"`
	Algorithm         string   `json:"algorithm"` // smartcrusher, codecompressor, kompress-base, ccr
	DurationMs        int64    `json:"duration_ms"`
	RetrievalKeys     []string `json:"retrieval_keys,omitempty"` // For CCR reversible compression
}

// Stats provides headroom performance metrics
type Stats struct {
	TotalRequests         int       `json:"total_requests"`
	TotalCompressed       int       `json:"total_compressed"`
	TotalOriginalTokens   int64     `json:"total_original_tokens"`
	TotalCompressedTokens int64     `json:"total_compressed_tokens"`
	AverageSavings        float64   `json:"average_savings_percent"`
	CacheHitRate          float64   `json:"cache_hit_rate"`
	LastLearnTime         time.Time `json:"last_learn_time,omitempty"`
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Enabled:           false,
		Mode:              ModeMCP,
		ProxyURL:          "http://localhost:8787/v1",
		CompressionLevel:  "normal",
		LearnFromFailures: true,
		StatsEnabled:      true,
		Timeout:           30 * time.Second,
		CacheEnabled:      true,
	}
}

// Constants for environment variables
const (
	EnvEnabled           = "HEADROOM_ENABLED"
	EnvMode              = "HEADROOM_MODE"
	EnvProxyURL          = "HEADROOM_PROXY_URL"
	EnvCompressionLevel  = "HEADROOM_COMPRESSION_LEVEL"
	EnvLearnFromFailures = "HEADROOM_LEARN"
	EnvStatsEnabled      = "HEADROOM_STATS"
	EnvTimeout           = "HEADROOM_TIMEOUT"
	EnvCacheEnabled      = "HEADROOM_CACHE"
)
