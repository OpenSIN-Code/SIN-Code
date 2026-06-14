# RTK Integration Guide for SIN-Code

## Overview

RTK (Rapid Toolkit) is an external tool that provides powerful capabilities for code analysis, linting, formatting, and testing. This integration brings RTK into SIN-Code with automatic detection, intelligent caching, and token optimization.

## Features

### Automatic Detection
- Auto-detects RTK binary in system paths
- Supports custom paths via configuration
- Fallback to environment variables
- Zero-configuration for most users

### Token Optimization
- ANSI color code stripping (60-90% token reduction)
- Intelligent caching with TTL
- Token counting and tracking
- Cost estimation and reporting

### Integration Levels

1. **CLI**: `sin-code rtk` commands
2. **Chat**: Auto-use in Agent Loop
3. **MCP**: Registered as MCP tool
4. **Programmatic**: Direct Go API

## Quick Start

### Detect RTK Installation

```bash
sin-code rtk detect
```

Output:
```
RTK Binary Found:
  Path:    /usr/local/bin/rtk
  Version: 3.1.0
  Method:  path_search
  Time:    12.5ms
```

### Run RTK Tools

```bash
sin-code rtk run lint file.go
sin-code rtk run format src/
sin-code rtk run test ./...
sin-code rtk run analyze --json
```

### Check Status

```bash
sin-code rtk status
```

Output:
```
RTK Status:
  Enabled:             true
  Binary Found:        true
  Binary Path:         /usr/local/bin/rtk
  Binary Version:      3.1.0
  Cache Enabled:       true
  Cache TTL:           24h0m0s
  Status:              ✅ Ready
```

### View Configuration

```bash
sin-code rtk config show
```

### Set Configuration

```bash
sin-code rtk config set cache_ttl 48h
sin-code rtk config set global_timeout 60s
sin-code rtk config set strip_ansi true
```

### View Metrics

```bash
sin-code rtk metrics
```

Output:
```
RTK Metrics:
  Total Executions:    145
  Successful:          142
  Failed:              3
  Cache Hits:          89
  Cache Misses:        56
  Total Duration:      3m42s
  Average Duration:    1.533s
  Tokens Saved:        45892
  Token Reduction:     78.3%
  Last Execution:      2024-06-14 15:23:45
```

## Configuration

### File Location

Configuration is stored at:
- Linux/Mac: `~/.config/rtk/rtk.json`
- Windows: `%APPDATA%\rtk\rtk.json`
- Custom: Set `RTK_CONFIG_DIR` environment variable

### Configuration Structure

```json
{
  "enabled": true,
  "binaryPath": "/usr/local/bin/rtk",
  "detectBinary": true,
  "globalTimeout": "60s",
  "cacheEnabled": true,
  "cacheTTL": "24h",
  "cacheDir": "",
  "stripANSI": true,
  "metricsEnabled": true,
  "logLevel": "info",
  "executionMode": "local",
  "mcpServerAddress": ""
}
```

### Environment Variables

```bash
# Binary path
export RTK_BINARY=/usr/local/bin/rtk

# Configuration directory
export RTK_CONFIG_DIR=~/.config/rtk

# Disable RTK
export RTK_DISABLED=true

# Log level
export RTK_LOG_LEVEL=debug
```

## API Usage

### Creating an Executor

```go
config := &rtk.RTKConfig{
    Enabled:       true,
    DetectBinary:  true,
    CacheEnabled:  true,
    StripANSI:     true,
    GlobalTimeout: 30 * time.Second,
}

executor, err := rtk.NewSimpleExecutor(config)
if err != nil {
    log.Fatal(err)
}
```

### Executing a Tool

```go
tool := &rtk.RTKTool{
    Name:    "lint",
    Args:    []string{"lint", "file.go"},
    Timeout: 30 * time.Second,
}

result, err := executor.Execute(context.Background(), tool)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Status: %s\n", result.Status)
fmt.Printf("Output: %s\n", result.StdoutClean)
fmt.Printf("Tokens: %d\n", result.TokenCount)
```

### MCP Tool Registration

```go
handler := rtk.NewMCPToolHandler(executor)

handler.RegisterTool(&rtk.RTKTool{
    Name:        "rtk_lint",
    Kind:        rtk.RTKToolKindValidator,
    Description: "Run RTK linter",
    Args:        []string{"lint"},
    Timeout:     30 * time.Second,
    Enabled:     true,
})

// Get tool definitions for MCP
defs := handler.GetToolDefinitions()

// Execute through MCP
result, err := handler.CallTool(ctx, "rtk_lint", map[string]interface{}{
    "args": []string{"file.go"},
})
```

### Configuration Management

```go
// Load configuration
manager := rtk.NewConfigManager("")
config, err := manager.LoadConfig()

// Save configuration
config.CacheTTL = 48 * time.Hour
manager.SaveConfig(config)

// Reset to defaults
manager.ResetConfig()
```

## Spec Layer Integration

RTK can analyze and enrich specifications:

```go
integration := rtk.NewSpecIndexingIntegration(executor)

// Enrich spec with RTK analysis
enrichment, err := integration.EnrichSpecWithRTKAnalysis(
    ctx, 
    "spec_auth_001", 
    specContent,
)

// Get token reduction report
report := integration.CalculateTokenReductionReport()
fmt.Printf("Tokens saved: %d (%.1f%%)\n", 
    report.TotalSaved, 
    report.ReductionPercent,
)
```

## Auto-Detection in Agent Loop

RTK can be automatically detected and used:

```go
// Enable auto RTK in Agent Loop
err := rtk.EnableAutoRTKInAgentLoop(context.Background())
if err != nil {
    log.Warn("RTK not available, continuing without")
}
```

## Error Handling

### Common Errors

| Error | Meaning | Solution |
|-------|---------|----------|
| `RTK_BINARY_NOT_FOUND` | RTK binary not detected | Install RTK or set `RTK_BINARY` env var |
| `RTK_EXECUTION_FAILED` | Tool execution failed | Check tool arguments and permissions |
| `RTK_TIMEOUT` | Tool exceeded timeout | Increase timeout or check RTK performance |
| `RTK_INVALID_CONFIG` | Configuration error | Validate configuration file |
| `RTK_CACHE_ERROR` | Cache operation failed | Check cache directory permissions |

### Fallback Strategies

```go
integration := rtk.NewSpecIndexingIntegration(executor)

// Strict error (fail immediately)
result, err := integration.ExecuteWithFallback(
    ctx, tool, rtk.FallbackStrictError,
)

// Graceful skip (skip RTK, continue)
result, err := integration.ExecuteWithFallback(
    ctx, tool, rtk.FallbackGracefulSkip,
)

// Retry with delay
result, err := integration.ExecuteWithFallback(
    ctx, tool, rtk.FallbackRetryWithDelay,
)

// Use cached result
result, err := integration.ExecuteWithFallback(
    ctx, tool, rtk.FallbackUseCachedResult,
)
```

## Performance

### Expected Performance

| Operation | Time | Notes |
|-----------|------|-------|
| Binary detection | 100-500ms | First time, cached after |
| Tool execution | Varies | Depends on tool and input |
| ANSI stripping | 50-200µs | Per result |
| Cache lookup | ~1µs | Negligible |
| Token reduction | 60-90% | Typical with ANSI stripping |

### Optimization Tips

1. **Enable Caching**: Reduces redundant RTK calls
2. **Enable ANSI Stripping**: Saves 60-90% tokens
3. **Batch Operations**: Group related tools
4. **Reuse Executor**: Don't recreate unnecessarily
5. **Use Appropriate Timeout**: Balance speed vs reliability

## Testing

### Run Tests

```bash
go test ./internal/rtk -v
```

### Run Benchmarks

```bash
go test ./internal/rtk -bench=. -benchmem
```

### Test Coverage

```bash
go test ./internal/rtk -cover
go test ./internal/rtk -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Troubleshooting

### RTK Not Detected

```bash
# Check if installed
which rtk

# Check version
rtk --version

# Try manual path
sin-code rtk --binary=/path/to/rtk status
```

### Slow Performance

```bash
# Check timeout
sin-code rtk config get global_timeout

# Increase if needed
sin-code rtk config set global_timeout 120s

# Check metrics
sin-code rtk metrics
```

### Cache Issues

```bash
# Clear cache in config
sin-code rtk config set cache_enabled false

# Check cache directory
echo $RTK_CONFIG_DIR

# Reset configuration
sin-code rtk config show  # Check current
```

## Integration with Other SIN-Code Components

### Spec Layer

RTK can analyze specs:
```bash
sin-code spec list | xargs -I {} sin-code rtk analyze {}
```

### Agent Loop

RTK is automatically available in chat:
```
User: Analyze this code
Bot: [Uses RTK automatically]
```

### MCP Server

RTK is registered as MCP tool:
```
MCP Tool: rtk_lint
MCP Tool: rtk_format
MCP Tool: rtk_test
MCP Tool: rtk_analyze
```

## References

- Issue #123: RTK Integration
- RTK Documentation: https://github.com/rtk/rtk
- SIN-Code: https://github.com/OpenSIN-Code/SIN-Code
