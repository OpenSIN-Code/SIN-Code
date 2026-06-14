// internal/headroom/client.go
package headroom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CLIClient invokes the headroom CLI directly.
type CLIClient struct {
	config Config
}

// NewCLIClient creates a new CLI-based headroom client.
func NewCLIClient(cfg Config) *CLIClient {
	return &CLIClient{config: cfg}
}

// Compress sends content to headroom compress command.
func (c *CLIClient) Compress(ctx context.Context, content string) (*CompressionResult, error) {
	start := time.Now()

	args := []string{"compress"}
	switch c.config.CompressionLevel {
	case "light":
		args = append(args, "--light")
	case "aggressive":
		args = append(args, "--aggressive")
	}
	// Add content via stdin
	cmd := exec.CommandContext(ctx, "headroom", args...)
	cmd.Stdin = strings.NewReader(content)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("headroom compress failed: %w, stderr: %s", err, stderr.String())
	}

	compressed := strings.TrimSpace(stdout.String())
	if compressed == "" {
		// Fallback: return original if headroom didn't output anything
		compressed = content
	}

	// Estimate token savings (rough approximation: 1 token ~4 chars)
	originalTokens := len(content) / 4
	compressedTokens := len(compressed) / 4
	savings := 0.0
	if originalTokens > 0 {
		savings = (1 - float64(compressedTokens)/float64(originalTokens)) * 100
	}

	return &CompressionResult{
		OriginalContent:   content,
		CompressedContent: compressed,
		OriginalTokens:    originalTokens,
		CompressedTokens:  compressedTokens,
		SavingsPercent:    savings,
		Algorithm:         "cli-compress",
		DurationMs:        time.Since(start).Milliseconds(),
	}, nil
}

// Learn invokes headroom learn on a failed session log.
func (c *CLIClient) Learn(ctx context.Context, sessionLog string) error {
	cmd := exec.CommandContext(ctx, "headroom", "learn")
	cmd.Stdin = strings.NewReader(sessionLog)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("headroom learn failed: %w, stderr: %s", err, stderr.String())
	}
	return nil
}

// Stats retrieves headroom performance stats.
func (c *CLIClient) Stats(ctx context.Context) (*Stats, error) {
	cmd := exec.CommandContext(ctx, "headroom", "stats", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("headroom stats failed: %w", err)
	}
	var stats Stats
	if err := json.Unmarshal(out, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse headroom stats: %w", err)
	}
	return &stats, nil
}

// Check verifies headroom CLI is available.
func (c *CLIClient) Check(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "headroom", "--version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("headroom CLI not found or not working: %w", err)
	}
	return nil
}
