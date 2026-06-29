// SPDX-License-Identifier: MIT
// Purpose: friendlyError wraps raw API/HTTP errors into user-facing
// messages with actionable hints (config commands, env vars, common
// models/URLs). Called from chat_run.go on loop.Run error paths.
package main

import (
	"fmt"
	"strings"
)

// friendlyError converts raw API/HTTP errors into user-friendly messages.
func friendlyError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()

	// HTTP 404 — model not found
	if strings.Contains(msg, "404") || strings.Contains(msg, "not found") {
		if strings.Contains(msg, "model") || strings.Contains(msg, "models/") {
			return fmt.Errorf("Model not found. Check your model setting:\n  sin-code config show\n  sin-code config set llm.model <model>\n\nCommon models:\n  nvidia/nemotron-3-nano-30b-a3b  (default, free)\n  nvidia/llama-3.3-nemotron-super-49b-v1\n  deepseek-ai/deepseek-r1\n\nOriginal error: %v", err)
		}
		return fmt.Errorf("Resource not found (404). Check your base URL and API key.\n  sin-code config show\n\nOriginal error: %v", err)
	}

	// HTTP 401/403 — auth errors
	if strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden") {
		return fmt.Errorf("Authentication failed. Your API key may be invalid or expired.\n  Check: sin-code config show\n  Set:  sin-code config set llm.api_key <key>\n  Or:   export NVIDIA_API_KEY=...\n\nOriginal error: %v", err)
	}

	// HTTP 429 — rate limited
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") {
		return fmt.Errorf("Rate limited (429). Please wait a moment and try again.\n\nOriginal error: %v", err)
	}

	// HTTP 500/502/503 — server errors
	if strings.Contains(msg, "500") || strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "internal server error") {
		return fmt.Errorf("LLM provider error (5xx). The service may be temporarily unavailable.\nTry again in a moment, or switch models:\n  sin-code config set llm.model <different-model>\n\nOriginal error: %v", err)
	}

	// Connection refused / timeout
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") || strings.Contains(msg, "no such host") {
		return fmt.Errorf("Cannot connect to LLM API. Check your base URL:\n  sin-code config show\n  sin-code config set llm.base_url <url>\n\nCommon base URLs:\n  https://integrate.api.nvidia.com/v1  (NVIDIA NIM, default)\n  https://api.fireworks.ai/inference/v1  (Fireworks AI)\n\nOriginal error: %v", err)
	}

	// No API key — already friendly from chat_run.go
	if strings.Contains(msg, "no LLM API key") {
		return err
	}

	return err
}
