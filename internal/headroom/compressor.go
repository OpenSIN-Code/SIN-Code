// internal/headroom/compressor.go
package headroom

import (
	"context"
	"fmt"
	"sync/atomic"
)

// Compressor is the main entry point for headroom compression in SIN-Code.
// It automatically selects the best available mode (MCP > CLI > disabled).
type Compressor struct {
	config   Config
	mode     Mode
	cliCli   *CLIClient
	mcpCli   *MCPClient
	lessons  *LessonStore
	enabled  bool
	stats    atomic.Value // stores *Stats
}

// SetLessonStore attaches a lesson store so that LearnFromFailure persists
// structured lessons in addition to forwarding to the headroom backend.
func (c *Compressor) SetLessonStore(store *LessonStore) {
	c.lessons = store
}

// Lessons returns the attached lesson store (may be nil).
func (c *Compressor) Lessons() *LessonStore {
	return c.lessons
}

// NewCompressor creates a compressor based on config.
// It does not start any background processes; call Start() to initialize.
func NewCompressor(cfg Config) *Compressor {
	c := &Compressor{
		config:  cfg,
		enabled: cfg.Enabled,
		mode:    cfg.Mode,
	}
	c.stats.Store(&Stats{})
	return c
}

// Start initializes the compressor. Returns error if headroom is not available.
func (c *Compressor) Start(ctx context.Context) error {
	if !c.enabled {
		return nil
	}

	switch c.mode {
	case ModeMCP:
		if err := c.startMCP(ctx); err != nil {
			// Fall back to CLI if the MCP server cannot be started.
			c.mode = ModeCLI
			return c.startCLI(ctx)
		}
		return nil
	case ModeCLI:
		return c.startCLI(ctx)
	case ModeProxy:
		// Proxy mode doesn't require a client here; handled by separate HTTP proxy setup.
		return nil
	default:
		return fmt.Errorf("unknown headroom mode: %s", c.mode)
	}
}

func (c *Compressor) startMCP(ctx context.Context) error {
	c.mcpCli = NewMCPClient(c.config)
	if err := c.mcpCli.Start(ctx); err != nil {
		c.mcpCli = nil
		return fmt.Errorf("headroom MCP start failed: %w", err)
	}
	return nil
}

func (c *Compressor) startCLI(ctx context.Context) error {
	c.cliCli = NewCLIClient(c.config)
	if err := c.cliCli.Check(ctx); err != nil {
		c.enabled = false
		return fmt.Errorf("headroom CLI check failed, disabling: %w", err)
	}
	return nil
}

// CompressContent compresses the given content. Returns the compressed string and result metadata.
func (c *Compressor) CompressContent(ctx context.Context, content string) (string, *CompressionResult, error) {
	if !c.enabled || content == "" {
		return content, nil, nil
	}

	var result *CompressionResult
	var err error

	switch c.mode {
	case ModeMCP:
		if c.mcpCli == nil {
			return content, nil, fmt.Errorf("MCP client not initialized")
		}
		result, err = c.mcpCli.Compress(ctx, content)
	case ModeCLI:
		if c.cliCli == nil {
			return content, nil, fmt.Errorf("CLI client not initialized")
		}
		result, err = c.cliCli.Compress(ctx, content)
	case ModeProxy:
		// In proxy mode, compression happens at the HTTP proxy layer.
		// Here we just pass through.
		return content, nil, nil
	default:
		return content, nil, nil
	}

	if err != nil {
		// On error, fall back to original content but log
		return content, nil, fmt.Errorf("compression failed, using original: %w", err)
	}

	// Update stats
	if result != nil {
		c.updateStats(result)
	}

	return result.CompressedContent, result, nil
}

// LearnFromFailure sends a failed session log to headroom learn and, when a
// lesson store is attached, records a structured lesson for future runs.
func (c *Compressor) LearnFromFailure(ctx context.Context, sessionLog string) error {
	if !c.enabled || !c.config.LearnFromFailures {
		return nil
	}

	// Persist a structured lesson locally regardless of backend availability.
	if c.lessons != nil && sessionLog != "" {
		c.lessons.Record("failure", truncatePattern(sessionLog), "session failed; preserve related context", 0.8)
		if err := c.lessons.Save(); err != nil {
			return fmt.Errorf("saving lesson: %w", err)
		}
	}

	switch c.mode {
	case ModeMCP:
		if c.mcpCli != nil {
			return c.mcpCli.Learn(ctx, sessionLog)
		}
		return nil
	case ModeCLI:
		if c.cliCli != nil {
			return c.cliCli.Learn(ctx, sessionLog)
		}
		return nil
	default:
		return nil
	}
}

// truncatePattern keeps lesson patterns compact so the store stays small.
func truncatePattern(s string) string {
	const max = 280
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// GetStats returns current headroom statistics.
func (c *Compressor) GetStats() *Stats {
	val := c.stats.Load()
	if val == nil {
		return &Stats{}
	}
	return val.(*Stats)
}

func (c *Compressor) updateStats(res *CompressionResult) {
	old := c.GetStats()
	newStats := &Stats{
		TotalRequests:         old.TotalRequests + 1,
		TotalCompressed:       old.TotalCompressed + 1,
		TotalOriginalTokens:   old.TotalOriginalTokens + int64(res.OriginalTokens),
		TotalCompressedTokens: old.TotalCompressedTokens + int64(res.CompressedTokens),
	}
	if newStats.TotalOriginalTokens > 0 {
		newStats.AverageSavings = (1 - float64(newStats.TotalCompressedTokens)/float64(newStats.TotalOriginalTokens)) * 100
	}
	newStats.LastLearnTime = old.LastLearnTime
	c.stats.Store(newStats)
}

// Close cleans up resources.
func (c *Compressor) Close() error {
	if c.mcpCli != nil {
		return c.mcpCli.Close()
	}
	return nil
}
