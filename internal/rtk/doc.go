package rtk

/*
Package rtk provides integration with the RTK (Rapid Toolkit) for SIN-Code.

# Overview

RTK is an external tool that performs various operations (linting, formatting, testing, analysis).
This package provides:

- Binary detection and execution
- Result caching with token optimization
- ANSI color code stripping (60-90% token reduction)
- MCP tool integration
- CLI command interface
- Configuration management
- Metrics collection

# Quick Start

	// Create executor
	config := &RTKConfig{
		Enabled:       true,
		DetectBinary:  true,
		CacheEnabled:  true,
		StripANSI:     true,
	}

	executor, err := NewSimpleExecutor(config)
	if err != nil {
		log.Fatal(err)
	}

	// Execute tool
	tool := &RTKTool{
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

# Features

## Binary Detection

Automatic detection of RTK binary in system paths:
- /usr/local/bin/rtk
- /usr/bin/rtk
- ~/.local/bin/rtk
- Custom configured path

## Result Caching

In-memory cache with configurable TTL:
- Automatic expiration
- Cache size limits
- Token-aware caching

## Token Optimization

ANSI color code stripping reduces tokens by 60-90%:
- Automatic detection and removal
- Clean and raw output variants
- Token count tracking

## MCP Integration

Register RTK tools as MCP tools:
- Tool definitions
- MCP-compatible output
- Multi-tool composition

## CLI Commands

Command-line interface for:
- Detecting RTK binary
- Running commands
- Showing/setting configuration
- Displaying metrics
- System status

## Configuration

Persistent configuration with:
- Environment variable overrides
- JSON file storage
- Validation
- Default values

## Metrics

Track performance with:
- Execution counts
- Success/failure rates
- Cache statistics
- Token savings
- Duration tracking

# Advanced Usage

## Custom Tools

	handler := NewMCPToolHandler(executor)

	handler.RegisterTool(&RTKTool{
		Name:        "custom_lint",
		Kind:        RTKToolKindValidator,
		Description: "Custom linting",
		Args:        []string{"custom", "lint"},
		Timeout:     45 * time.Second,
		Enabled:     true,
	})

## Multiple Execution Modes

	// Local execution
	config.ExecutionMode = RTKExecutionModeLocal

	// MCP server execution
	config.ExecutionMode = RTKExecutionModeMCP
	config.MCPServerAddress = "localhost:8080"

	// Remote execution
	config.ExecutionMode = RTKExecutionModeRemote

## Retry Policy

	retryPolicy := &RetryPolicy{
		MaxRetries:        3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		RetryableErrors:   []string{"timeout", "connection"},
	}

	config.RetryPolicy = retryPolicy

# Performance

Expected performance with token optimization:

	- RTK binary detection: ~100-500ms
	- Tool execution: Depends on tool (typically 100ms-10s)
	- ANSI stripping: ~50-200µs per result
	- Token reduction: 60-90%
	- Cache lookup: ~1µs

# Integration with SIN-Code

RTK integrates at multiple levels:

	1. Agent Loop: Auto-use RTK when available
	2. Chat: /rtk commands
	3. MCP Server: RTK tools registered
	4. CLI: sin-code rtk commands

# Error Handling

Common errors:

	- ErrBinaryNotFound: RTK binary not found in system
	- ErrExecutionFailed: Tool execution failed
	- ErrTimeout: Tool execution exceeded timeout
	- ErrInvalidConfig: Configuration error

*/

import "time"
