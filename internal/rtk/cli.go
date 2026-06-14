package rtk

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
)

// CLICommand handles RTK CLI commands
type CLICommand struct {
	executor RTKExecutor
	config   *RTKConfig
}

// NewCLICommand creates a new CLI command handler
func NewCLICommand(executor RTKExecutor) *CLICommand {
	if executor == nil {
		return nil
	}

	return &CLICommand{
		executor: executor,
		config:   executor.GetConfig(),
	}
}

// RunCommand executes a command line tool
func (c *CLICommand) RunCommand(ctx context.Context, name string, args []string) error {
	tool := &RTKTool{
		Name:    name,
		Args:    args,
		Timeout: c.config.GlobalTimeout,
	}

	result, err := c.executor.Execute(ctx, tool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return err
	}

	// Output results
	c.printResult(result)

	if result.Status != RTKStatusSuccess {
		return fmt.Errorf("command failed: %s", result.Name)
	}

	return nil
}

// DetectCommand detects RTK binary
func (c *CLICommand) DetectCommand(ctx context.Context) error {
	info, err := c.executor.Detect(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "RTK binary not found\n")
		return err
	}

	fmt.Printf("RTK Binary Found:\n")
	fmt.Printf("  Path:    %s\n", info.Path)
	fmt.Printf("  Version: %s\n", info.Version)
	fmt.Printf("  Method:  %s\n", info.Method)
	fmt.Printf("  Time:    %v\n", info.DetectionTime)

	return nil
}

// ConfigCommand shows or sets configuration
func (c *CLICommand) ConfigCommand(subcommand string, key string, value string) error {
	switch subcommand {
	case "show":
		return c.showConfig()
	case "set":
		return c.setConfigValue(key, value)
	case "get":
		return c.getConfigValue(key)
	default:
		return fmt.Errorf("unknown config subcommand: %s", subcommand)
	}
}

// showConfig displays current configuration
func (c *CLICommand) showConfig() error {
	data, err := json.MarshalIndent(c.config, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// setConfigValue sets a configuration value
func (c *CLICommand) setConfigValue(key, value string) error {
	switch key {
	case "enabled":
		c.config.Enabled = value == "true"
	case "detect_binary":
		c.config.DetectBinary = value == "true"
	case "strip_ansi":
		c.config.StripANSI = value == "true"
	case "cache_enabled":
		c.config.CacheEnabled = value == "true"
	case "cache_ttl":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration: %s", value)
		}
		c.config.CacheTTL = duration
	case "global_timeout":
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration: %s", value)
		}
		c.config.GlobalTimeout = duration
	case "log_level":
		c.config.LogLevel = value
	case "binary_path":
		c.config.BinaryPath = value
	case "execution_mode":
		c.config.ExecutionMode = RTKExecutionMode(value)
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	fmt.Printf("Config updated: %s = %s\n", key, value)
	return nil
}

// getConfigValue gets a configuration value
func (c *CLICommand) getConfigValue(key string) error {
	var value interface{}

	switch key {
	case "enabled":
		value = c.config.Enabled
	case "detect_binary":
		value = c.config.DetectBinary
	case "strip_ansi":
		value = c.config.StripANSI
	case "cache_enabled":
		value = c.config.CacheEnabled
	case "cache_ttl":
		value = c.config.CacheTTL.String()
	case "global_timeout":
		value = c.config.GlobalTimeout.String()
	case "log_level":
		value = c.config.LogLevel
	case "binary_path":
		value = c.config.BinaryPath
	case "execution_mode":
		value = c.config.ExecutionMode
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	fmt.Printf("%s: %v\n", key, value)
	return nil
}

// MetricsCommand displays metrics
func (c *CLICommand) MetricsCommand() error {
	metrics := c.executor.GetMetrics()

	fmt.Println("RTK Metrics:")
	fmt.Printf("  Total Executions:    %d\n", metrics.TotalExecutions)
	fmt.Printf("  Successful:          %d\n", metrics.SuccessfulCalls)
	fmt.Printf("  Failed:              %d\n", metrics.FailedCalls)
	fmt.Printf("  Cache Hits:          %d\n", metrics.CacheHits)
	fmt.Printf("  Cache Misses:        %d\n", metrics.CacheMisses)
	fmt.Printf("  Total Duration:      %v\n", metrics.TotalDuration)
	fmt.Printf("  Average Duration:    %v\n", metrics.AverageDuration)
	fmt.Printf("  Tokens Saved:        %d\n", metrics.TokensSaved)
	fmt.Printf("  Token Reduction:     %.1f%%\n", metrics.TokenReduction*100)
	fmt.Printf("  Last Execution:      %v\n", metrics.LastExecution)
	if metrics.LastError != "" {
		fmt.Printf("  Last Error:          %s\n", metrics.LastError)
	}

	return nil
}

// StatusCommand shows RTK status
func (c *CLICommand) StatusCommand(ctx context.Context) error {
	fmt.Println("RTK Status:")

	// Check if enabled
	fmt.Printf("  Enabled:             %v\n", c.config.Enabled)

	// Check binary
	info, err := c.executor.Detect(ctx)
	if err != nil {
		fmt.Printf("  Binary Found:        false\n")
		fmt.Printf("  Status:              ❌ RTK binary not found\n")
		return nil
	}

	fmt.Printf("  Binary Found:        true\n")
	fmt.Printf("  Binary Path:         %s\n", info.Path)
	fmt.Printf("  Binary Version:      %s\n", info.Version)

	// Check cache
	fmt.Printf("  Cache Enabled:       %v\n", c.config.CacheEnabled)
	fmt.Printf("  Cache TTL:           %v\n", c.config.CacheTTL)

	// Show overall status
	if c.config.Enabled && info.Path != "" {
		fmt.Printf("  Status:              ✅ Ready\n")
	} else {
		fmt.Printf("  Status:              ⚠️  Not ready\n")
	}

	return nil
}

// printResult displays an RTK result
func (c *CLICommand) printResult(result *RTKResult) {
	fmt.Printf("Command: %s\n", result.Name)
	fmt.Printf("Status:  %s\n", result.Status)
	fmt.Printf("Exit:    %d\n", result.ExitCode)
	fmt.Printf("Time:    %v\n", result.Duration)

	if result.Cached {
		fmt.Printf("Cached:  true (at %v)\n", result.CachedAt)
	}

	fmt.Printf("Tokens:  %d\n", result.TokenCount)

	if result.Stdout != "" {
		fmt.Println("\nOutput:")
		if c.config.StripANSI {
			fmt.Println(result.StdoutClean)
		} else {
			fmt.Println(result.Stdout)
		}
	}

	if result.Stderr != "" {
		fmt.Println("\nErrors:")
		if c.config.StripANSI {
			fmt.Println(result.StderrClean)
		} else {
			fmt.Println(result.Stderr)
		}
	}
}

// Logger interface for customizable logging
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// DefaultLogger provides basic logging
type DefaultLogger struct {
	level string
}

// NewDefaultLogger creates a default logger
func NewDefaultLogger(level string) *DefaultLogger {
	return &DefaultLogger{level: level}
}

func (l *DefaultLogger) Debug(msg string, args ...interface{}) {
	if l.level == "debug" {
		log.Printf("[DEBUG] "+msg, args...)
	}
}

func (l *DefaultLogger) Info(msg string, args ...interface{}) {
	if l.level == "debug" || l.level == "info" {
		log.Printf("[INFO] "+msg, args...)
	}
}

func (l *DefaultLogger) Warn(msg string, args ...interface{}) {
	if l.level != "error" {
		log.Printf("[WARN] "+msg, args...)
	}
}

func (l *DefaultLogger) Error(msg string, args ...interface{}) {
	log.Printf("[ERROR] "+msg, args...)
}
