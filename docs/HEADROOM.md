# Headroom Integration in SIN-Code

## Overview

Headroom provides intelligent context compression for LLM requests. This integration allows SIN-Code to reduce token usage by up to 92% with minimal accuracy loss, saving costs and improving response times.

## Architecture

### Components

1. **`internal/headroom/types.go`** - Type definitions (Config, Mode, CompressionResult, Stats)
2. **`internal/headroom/client.go`** - CLI-based Headroom client
3. **`internal/headroom/config.go`** - Configuration loading from environment variables
4. **`internal/headroom/compressor.go`** - Main compression orchestrator
5. **`internal/agentloop/headroom_hook.go`** - Integration hook for Agent Loop
6. **`cmd/sin-code/headroom_cmd.go`** - CLI commands for SIN-Code

### Modes

- **CLI Mode**: Direct invocation of `headroom` command (pip install headroom-ai)
- **MCP Mode**: Native MCP tools (for future MCP SDK integration)
- **Proxy Mode**: HTTP proxy intercepts LLM API calls (zero code change)

## Installation

1. Install Headroom:
```bash
pip install headroom-ai
```

2. Enable in SIN-Code:
```bash
sin-code headroom enable
```

## Environment Variables

```bash
HEADROOM_ENABLED=true              # Enable/disable compression
HEADROOM_MODE=cli                  # cli, mcp, proxy
HEADROOM_COMPRESSION_LEVEL=normal  # light, normal, aggressive
HEADROOM_LEARN=true                # Learn from failures
HEADROOM_STATS=true                # Track statistics
HEADROOM_TIMEOUT=30s               # Compression timeout
HEADROOM_CACHE=true                # Enable compression cache
```

## CLI Commands

```bash
# Enable compression
sin-code headroom enable

# Disable compression
sin-code headroom disable

# Show statistics
sin-code headroom stats

# Test connection
sin-code headroom test

# Learn from a session log
sin-code headroom learn session.log
```

## Usage in Agent Loop

The `HeadroomHook` automatically compresses message content before sending to the LLM:

```go
cfg := headroom.LoadConfigFromEnv()
hook, err := agentloop.NewHeadroomHook(cfg)
if err == nil {
    messages, _ := hook.PreRequest(ctx, messages)
}
```

## Statistics

Track compression efficiency:

```bash
sin-code headroom stats
```

Output includes:
- Total requests processed
- Average token savings percentage
- Original vs compressed tokens
- Cache hit rate

## Learning

Headroom can learn from failed sessions to improve compression:

```bash
sin-code headroom learn failed_session.log
```

## Integration with Lessons Database

Failed sessions can be recorded and used to improve compression models via the HeadroomLearner interface.

## Performance

- Typical compression: 30-70% token reduction
- Aggressive compression: up to 92% token reduction
- Minimal accuracy loss with intelligent algorithms
- SmartCrusher, CodeCompressor, Kompress-base algorithms

## Configuration File

See `docs/headroom_config.json` for an example configuration.

## Testing

Run unit tests:

```bash
go test ./internal/headroom/...
```

Test compression:

```bash
sin-code headroom test
```

## Troubleshooting

- **"headroom CLI not found"**: Install with `pip install headroom-ai`
- **Compression disabled silently**: Check `HEADROOM_ENABLED` env var
- **Stats show zero**: Run compression with `sin-code headroom test`
- **Learning not working**: Ensure `HEADROOM_LEARN=true` and headroom is installed

## References

- Headroom Documentation: https://github.com/lgrammel/headroom
- Issue #118: Full integration specification
