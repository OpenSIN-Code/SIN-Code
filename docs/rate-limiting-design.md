# SIN-Code MCP Server Rate-Limiting Design

## Overview

The SIN-Code MCP server handles tool execution requests from multiple clients concurrently. Rate limiting is essential to prevent abuse, ensure fair resource allocation, and protect the host system from overload. This design document specifies a multi-tier, token-bucket-based rate-limiting system integrated into the MCP server’s request pipeline.

## Goals

- **Fairness**: Prevent a single client from monopolizing server resources.
- **Stability**: Protect the server from overload (e.g., excessive `execute`, `harvest`, or plugin calls).
- **Granularity**: Apply different limits per client, per tool, and globally based on resource cost.
- **Transparency**: Provide clear, structured error responses when rate limits are hit, with retry-after guidance.
- **Observability**: Expose metrics for rate-limit events, queue depth, and current utilization.
- **Zero Dependencies**: No external services (Redis, etcd) required; all state is in-memory.

## Non-Goals

- Distributed rate limiting across multiple server instances (out of scope for v1; can be extended later).
- User-based authentication or billing (client identity is process-based or token-based only).
- Hard real-time guarantees (best-effort within milliseconds).

---

## Rate-Limiting Tiers

The system applies three independent tiers of rate limiting. A request is allowed only if it passes **all** applicable tiers.

### 1. Global Rate Limiting

Protects the overall server from overload.

| Parameter | Default | Description |
|-----------|---------|-------------|
| `requests_per_minute` | 120 | Total allowed requests across all clients per minute. |
| `burst` | 10 | Maximum burst of requests allowed in a short window (1 second). |
| `window` | 60s | The primary time window for `requests_per_minute`. |

**Behavior**: If global limits are exceeded, the server returns `429 Too Many Requests` to all clients, regardless of their individual usage. This is the last line of defense against cascading overload.

### 2. Per-Client Rate Limiting

Tracks requests per client identity. A client is identified by:

1. **Process ID** (PID): Extracted from the transport metadata (stdio, SSE, or HTTP connection). This is the default for local clients.
2. **Auth Token**: If an API key or bearer token is provided, the client ID is derived from a hash of the token (SHA-256, truncated to 16 chars).
3. **Connection ID**: For WebSocket / SSE connections, a unique connection ID is assigned.

The server maintains a sliding counter per client ID. If no explicit identifier is available, the client falls back to a default `anonymous` bucket.

| Parameter | Default | Description |
|-----------|---------|-------------|
| `requests_per_minute` | 60 | Allowed requests per client per minute. |
| `burst` | 5 | Maximum burst per client in 1 second. |
| `max_clients` | 1000 | Maximum number of distinct client IDs tracked in memory. Oldest inactive clients are evicted via LRU. |

**Behavior**: When a client exceeds its limit, only that client receives `429`. Other clients are unaffected.

### 3. Per-Tool Rate Limiting

Some tools are significantly more expensive than others (e.g., `sin_execute` runs shell commands, `sin_harvest` fetches URLs, plugin tool calls spawn processes). Per-tool limits prevent expensive tools from being abused.

**Default Configuration**:

```json
{
  "global": {
    "requests_per_minute": 120,
    "burst": 10
  },
  "per_client": {
    "requests_per_minute": 60,
    "burst": 5
  },
  "per_tool": {
    "sin_execute": {
      "requests_per_minute": 30,
      "burst": 3,
      "cost": 2
    },
    "sin_harvest": {
      "requests_per_minute": 20,
      "burst": 2,
      "cost": 3
    },
    "sin_map": {
      "requests_per_minute": 40,
      "burst": 4,
      "cost": 1
    },
    "sin_discover": {
      "requests_per_minute": 60,
      "burst": 5,
      "cost": 1
    },
    "default": {
      "requests_per_minute": 60,
      "burst": 5,
      "cost": 1
    }
  }
}
```

**Cost Model**: Each tool invocation consumes a configurable `cost` from the token bucket. A tool with `cost: 2` consumes twice as many tokens as a `cost: 1` tool. This allows fine-grained weighting without adjusting rates directly.

**Tier Interaction**: The per-tool limit is checked against the **global** and **per-client** buckets. A tool call must pass all three tiers. The tool’s `cost` is deducted from each applicable bucket.

---

## Algorithm: Token Bucket

The token bucket algorithm is chosen for its simplicity, predictable burst behavior, and well-understood semantics.

### Data Structure

```go
type TokenBucket struct {
    capacity    float64   // Maximum tokens (burst)
    tokens      float64   // Current available tokens
    rate        float64   // Tokens added per second (requests_per_minute / 60)
    lastUpdated time.Time // Last time tokens were added
    mu          sync.Mutex
}
```

### Algorithm

```go
func (tb *TokenBucket) Allow(cost float64) (bool, time.Duration) {
    tb.mu.Lock()
    defer tb.mu.Unlock()

    now := time.Now()
    elapsed := now.Sub(tb.lastUpdated).Seconds()
    tb.tokens += elapsed * tb.rate
    if tb.tokens > tb.capacity {
        tb.tokens = tb.capacity
    }
    tb.lastUpdated = now

    if tb.tokens >= cost {
        tb.tokens -= cost
        return true, 0
    }

    // Calculate wait time until enough tokens are available
    needed := cost - tb.tokens
    wait := time.Duration(needed / tb.rate * float64(time.Second))
    return false, wait
}
```

### Why Token Bucket?

| Property | Token Bucket | Sliding Window | Fixed Window |
|----------|--------------|----------------|--------------|
| Burst handling | ✅ Exact | ❌ Approximate | ❌ None |
| Memory per client | O(1) | O(N) requests | O(1) |
| Implementation | Simple | Complex | Simple |
| Fairness | High | High | Low (edge effects) |

Token bucket provides a clean trade-off: simple implementation, exact burst control, and constant memory per client.

---

## Implementation Architecture

### Component Diagram

```
┌─────────────────────────────────────────┐
│           MCP Client Request            │
└─────────────────────┬───────────────────┘
                      │
        ┌─────────────▼─────────────┐
        │   Transport Layer (stdio,   │
        │   SSE, HTTP, WebSocket)     │
        └─────────────┬─────────────┘
                      │
        ┌─────────────▼─────────────┐
        │   Client ID Extractor     │
        │  (PID, Token, Conn ID)    │
        └─────────────┬─────────────┘
                      │
        ┌─────────────▼─────────────┐
        │   Rate Limit Middleware   │
        │  ┌─────────────────────┐  │
        │  │  Global Bucket      │  │
        │  │  (server-wide)      │  │
        │  └─────────────────────┘  │
        │  ┌─────────────────────┐  │
        │  │  Per-Client Bucket  │  │
        │  │  (clientID → bucket)│  │
        │  └─────────────────────┘  │
        │  ┌─────────────────────┐  │
        │  │  Per-Tool Bucket    │  │
        │  │  (toolName → bucket)│  │
        │  └─────────────────────┘  │
        └─────────────┬─────────────┘
                      │
              ┌───────┴───────┐
              │  Allowed?     │
              │  Yes → Execute │
              │  No  → 429    │
              └───────────────┘
```

### Core Structures (Go)

```go
package ratelimit

import (
    "sync"
    "time"
)

// Config holds the rate limit configuration.
type Config struct {
    Global    LimitConfig            `json:"global"`
    PerClient LimitConfig            `json:"per_client"`
    PerTool   map[string]ToolConfig `json:"per_tool"`
}

type LimitConfig struct {
    RequestsPerMinute int `json:"requests_per_minute"`
    Burst             int `json:"burst"`
}

type ToolConfig struct {
    LimitConfig
    Cost float64 `json:"cost,omitempty"`
}

// TokenBucket implements the token bucket algorithm.
type TokenBucket struct {
    capacity    float64
    tokens      float64
    rate        float64 // tokens per second
    lastUpdated time.Time
    mu          sync.Mutex
}

// Manager holds all buckets and enforces limits.
type Manager struct {
    config       Config
    global       *TokenBucket
    clients      map[string]*TokenBucket
    tools        map[string]*TokenBucket
    clientMu     sync.RWMutex
    toolMu       sync.RWMutex
    cleanupTicker *time.Ticker
    stopCh       chan struct{}
}
```

### Middleware Integration (`serve.go`)

The rate limiter is integrated as a **middleware layer** in the MCP server request handler, before tool dispatch:

```go
func (s *Server) handleToolCall(ctx context.Context, req *jsonrpc.Request) (*jsonrpc.Response, error) {
    clientID := extractClientID(ctx)
    toolName := req.Method // e.g., "sin_execute"

    // Check rate limits
    allowed, wait, err := s.rateLimiter.Allow(ctx, clientID, toolName)
    if err != nil {
        return nil, fmt.Errorf("rate limiter error: %w", err)
    }
    if !allowed {
        return nil, &RateLimitError{
            Message:    "Rate limit exceeded. Try again after " + wait.String(),
            RetryAfter: int(wait.Seconds()),
        }
    }

    // Proceed to tool execution
    return s.dispatchTool(ctx, req)
}
```

The `RateLimitError` is serialized to a JSON-RPC error with code `-32029` (custom, analogous to HTTP 429):

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "error": {
    "code": -32029,
    "message": "Rate limit exceeded. Try again after 5s",
    "data": {
      "retry_after_seconds": 5,
      "limit_type": "per_client",
      "limit": 60,
      "window": "1m"
    }
  }
}
```

---

## Configuration

### File-based Configuration

Rate limit configuration is stored in `~/.config/sin/rate_limits.json` (or the equivalent system path) and is reloaded via `SIGHUP` or an API call.

```json
{
  "global": {
    "requests_per_minute": 120,
    "burst": 10
  },
  "per_client": {
    "requests_per_minute": 60,
    "burst": 5
  },
  "per_tool": {
    "sin_execute": {
      "requests_per_minute": 30,
      "burst": 3,
      "cost": 2
    },
    "sin_harvest": {
      "requests_per_minute": 20,
      "burst": 2,
      "cost": 3
    },
    "sin_map": {
      "requests_per_minute": 40,
      "burst": 4,
      "cost": 1
    },
    "sin_discover": {
      "requests_per_minute": 60,
      "burst": 5,
      "cost": 1
    }
  }
}
```

### CLI Configuration

Users can adjust limits via the `sin-code config` CLI:

```bash
# View current limits
sin-code config get rate_limits

# Set global limit
sin-code config set rate_limits.global.requests_per_minute 120

# Set per-tool limit
sin-code config set rate_limits.per_tool.sin_execute.requests_per_minute 30
sin-code config set rate_limits.per_tool.sin_execute.cost 2

# Disable rate limiting (emergency / local dev)
sin-code config set rate_limits.enabled false
```

Changes are persisted to the config file and applied immediately (no server restart needed).

---

## Queue Management & Backpressure

When a request is rate-limited, the server has two strategies:

### 1. Fast Reject (Default)

Return `429` immediately. This is the default for the MCP server because:
- MCP clients are typically agents that can handle retry logic.
- Fast reject prevents head-of-line blocking.
- Memory usage is bounded (no queue accumulation).

### 2. Short Queue (Opt-in)

For critical, non-interactive clients, a small queue (default: 10 requests per client) can be enabled:

```bash
sin-code config set rate_limits.queue.enabled true
sin-code config set rate_limits.queue.max_per_client 10
sin-code config set rate_limits.queue.max_global 100
sin-code config set rate_limits.queue.timeout 30s
```

**Queue Behavior**:
- Requests are held in a FIFO queue until the token bucket allows them.
- If the queue exceeds `max_per_client` or `max_global`, the oldest request is dropped with `429`.
- If a request waits longer than `timeout`, it is dropped with `408 Request Timeout` (code `-32008`).
- Queue depth is exposed in metrics.

**Recommendation**: Use fast reject for agent clients (default). Use queue mode only for synchronous integrations that cannot handle retries.

---

## Memory Management & Cleanup

Since rate limit state is entirely in-memory, we must prevent unbounded growth.

### Client Bucket Eviction

- **Max clients**: Default 1000. When exceeded, the least recently used (LRU) client bucket is evicted.
- **Idle timeout**: Client buckets are removed after 10 minutes of inactivity.
- A background goroutine (interval: 1 minute) scans and evicts stale buckets.

### Tool Bucket Behavior

- Tool buckets are pre-allocated at startup for all configured tools (native + plugins).
- Dynamic tool buckets (e.g., newly loaded plugin tools) are added on first request and removed when the plugin is unloaded.

### Memory Footprint Estimate

```
Per bucket: ~64 bytes (TokenBucket struct + map overhead)
1000 clients + 50 tools = ~67 KB
```

Negligible for typical deployments.

---

## Metrics & Observability

The rate limiter exposes the following metrics via the existing stats endpoint (`/stats` or `tools/call` on `sin_stats`):

| Metric | Type | Description |
|--------|------|-------------|
| `rate_limit.allowed_total` | Counter | Total requests allowed. |
| `rate_limit.rejected_total` | Counter | Total requests rejected, tagged by `limit_type` (global, per_client, per_tool). |
| `rate_limit.queue_depth` | Gauge | Current queue depth (if queue enabled). |
| `rate_limit.queue_wait_ms` | Histogram | Time spent in queue before execution or rejection. |
| `rate_limit.global_tokens` | Gauge | Current tokens in global bucket. |
| `rate_limit.client_tokens` | Gauge | Current tokens per client (top 10 by usage). |
| `rate_limit.client_count` | Gauge | Number of active client buckets. |

**Example `/stats` output**:

```json
{
  "rate_limits": {
    "global": {
      "tokens": 8.5,
      "capacity": 10,
      "allowed": 450,
      "rejected": 12
    },
    "per_client": {
      "client_pid_12345": {
        "tokens": 3.2,
        "capacity": 5,
        "allowed": 45,
        "rejected": 1
      }
    },
    "per_tool": {
      "sin_execute": {
        "tokens": 1.0,
        "capacity": 3,
        "allowed": 20,
        "rejected": 2
      }
    }
  }
}
```

---

## Implementation Phases

### Phase 1: Core Token Bucket & Global Limit

- [ ] Implement `TokenBucket` struct and `Allow()` method in `internal/ratelimit/bucket.go`.
- [ ] Implement `Manager` struct with global bucket in `internal/ratelimit/manager.go`.
- [ ] Integrate into `serve.go` request handler: check global limit before tool dispatch.
- [ ] Return JSON-RPC error `-32029` with `retry_after` on rejection.
- [ ] Unit tests: burst behavior, token refill, edge cases (cost > capacity).
- [ ] Add `sin-code config set rate_limits.global.*` CLI support.

**Goal**: Global rate limiting is functional.

### Phase 2: Per-Client & Per-Tool Limits

- [ ] Implement client ID extraction (PID, token hash, connection ID) in `internal/ratelimit/client.go`.
- [ ] Implement per-client bucket map with LRU eviction and idle cleanup.
- [ ] Implement per-tool bucket map with cost-weighted deductions.
- [ ] Tier enforcement: all three buckets must pass.
- [ ] Add CLI configuration for per-client and per-tool limits.
- [ ] Unit tests: multi-client isolation, tool cost weighting, bucket eviction.

**Goal**: Multi-tier rate limiting is complete.

### Phase 3: Queue Management & Backpressure

- [ ] Implement optional FIFO queue per client.
- [ ] Implement global queue cap and timeout handling.
- [ ] Add `rate_limits.queue.enabled` config flag.
- [ ] Unit tests: queue overflow, timeout, FIFO ordering.

**Goal**: Queue mode is available for synchronous integrations.

### Phase 4: Metrics & Observability

- [ ] Integrate metrics into the existing `/stats` endpoint.
- [ ] Add Prometheus-compatible metric counters/gauges (if Prometheus export is enabled).
- [ ] Add structured logging for every rate-limit event (allowed/rejected).
- [ ] Add `sin_stats` tool call to query rate limit status.
- [ ] Unit tests: metric accuracy, stats endpoint correctness.

**Goal**: Rate limit behavior is fully observable.

### Phase 5: Dynamic Reload & Advanced Tuning

- [ ] Implement `SIGHUP` handler to reload config without restart.
- [ ] Implement `sin-code config set` hot-reload (apply immediately).
- [ ] Add adaptive rate limiting: if server CPU > 80%, automatically reduce global burst by 50% (optional, opt-in).
- [ ] Document tuning guide for high-load deployments.
- [ ] Integration tests: reload correctness, adaptive behavior.

**Goal**: Production-grade flexibility and tuning.

---

## Integration Points

### 1. Hook into `serve.go`

The rate limiter is a middleware function in the MCP server's `handleRequest` pipeline:

```go
func (s *Server) handleRequest(ctx context.Context, req *jsonrpc.Request) (*jsonrpc.Response, error) {
    if req.Method == "tools/call" || req.Method == "tools/list" {
        // Only rate-limit tool calls; skip metadata/health checks
        toolName := extractToolName(req)
        clientID := extractClientID(ctx)
        if ok, wait, err := s.rateLimiter.Allow(ctx, clientID, toolName); !ok {
            return nil, newRateLimitError(wait, err)
        }
    }
    return s.dispatch(ctx, req)
}
```

### 2. Structured Error to Client

When rate-limited, the client receives a JSON-RPC error object:

```json
{
  "jsonrpc": "2.0",
  "id": 42,
  "error": {
    "code": -32029,
    "message": "Rate limit exceeded",
    "data": {
      "type": "per_client",
      "retry_after_seconds": 5,
      "limit": 60,
      "window": "1m",
      "suggestion": "Wait 5s before retrying."
    }
  }
}
```

MCP clients (e.g., OpenCode) should recognize code `-32029` and implement exponential backoff with `retry_after` as the initial delay.

### 3. Logging

Every rate-limit event is logged with structured fields:

```json
{
  "time": "2025-01-01T12:00:00Z",
  "level": "WARN",
  "msg": "rate_limit_rejected",
  "client_id": "pid:12345",
  "tool": "sin_execute",
  "limit_type": "per_client",
  "retry_after_seconds": 5,
  "queue_depth": 0
}
```

---

## Open Questions & Decisions

| Question | Decision | Rationale |
|----------|----------|-----------|
| Algorithm: Token bucket vs. sliding window? | Token bucket | Simpler, exact burst control, O(1) memory. |
| Queue: Default on or off? | Off (fast reject) | MCP agents handle retry; queue adds complexity. |
| Client ID: PID vs. token? | PID for local, token hash for remote | PIDs are reliable for local stdio; tokens for remote auth. |
| Distributed rate limiting? | Out of scope (v1) | Single-process server assumption. Redis extension possible later. |
| Cost: Float vs. int? | Float (default 1.0) | Allows fine weighting (e.g., `cost: 1.5` for medium tools). |
| Bucket cleanup interval? | 1 minute | Trade-off between memory freshness and CPU overhead. |

---

## Appendix: Default Configuration

```json
{
  "rate_limits": {
    "enabled": true,
    "global": {
      "requests_per_minute": 120,
      "burst": 10
    },
    "per_client": {
      "requests_per_minute": 60,
      "burst": 5,
      "max_clients": 1000,
      "idle_timeout_minutes": 10
    },
    "per_tool": {
      "default": {
        "requests_per_minute": 60,
        "burst": 5,
        "cost": 1.0
      },
      "sin_execute": {
        "requests_per_minute": 30,
        "burst": 3,
        "cost": 2.0
      },
      "sin_harvest": {
        "requests_per_minute": 20,
        "burst": 2,
        "cost": 3.0
      },
      "sin_map": {
        "requests_per_minute": 40,
        "burst": 4,
        "cost": 1.0
      }
    },
    "queue": {
      "enabled": false,
      "max_per_client": 10,
      "max_global": 100,
      "timeout_seconds": 30
    }
  }
}
```

---

## Appendix: JSON-RPC Error Codes

| Code | Meaning | Usage |
|------|---------|-------|
| `-32029` | Rate limit exceeded | Per-client, per-tool, or global limit hit. |
| `-32008` | Queue timeout | Request timed out waiting in queue. |
| `-32600` | Invalid request | Malformed JSON-RPC request. |
| `-32601` | Method not found | Unknown tool name. |
