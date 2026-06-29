// SPDX-License-Identifier: MIT
// Purpose: bridges internal/llm.Client (STRUCT) to the agentloop.Completion
// func signature via a func-closure (issue #44). Adds OpenAI-compatible
// tool calling on top of the plain-chat client.
package agentloop

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

type wireFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireThinking struct {
	Type   string `json:"type,omitempty"`          // "enabled" | "disabled"
	Budget int    `json:"budget_tokens,omitempty"` // cap on per-request reasoning tokens
}

type wireRequest struct {
	Model       string            `json:"model"`
	Messages    []session.Message `json:"messages"`
	Tools       []wireTool        `json:"tools,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	Thinking    *wireThinking     `json:"thinking,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	ThinkingTokens   int `json:"thinking_tokens,omitempty"`
}

type wireResponse struct {
	ID      string `json:"id,omitempty"`
	Choices []struct {
		Message struct {
			Role      string         `json:"role"`
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage wireUsage `json:"usage,omitempty"`
}

// marshalToolCallsHook is swapped by coverage tests to exercise the JSON
// re-marshal error branch.
var marshalToolCallsHook func(v any) ([]byte, error)

func marshalToolCalls(v any) ([]byte, error) {
	if marshalToolCallsHook != nil {
		return marshalToolCallsHook(v)
	}
	return json.Marshal(v)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func NewProviderCompletion(c *llm.Client, model string, maxTokens int, temperature float64) func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
	return NewProviderCompletionWithCache(c, model, maxTokens, temperature, nil)
}

// ThinkingConfig is the wire-side configuration for the per-request
// "thinking" / internal reasoning budget a provider may honor (Claude
// / Anthropic on NIM-style gateways, OpenRouter, etc.). Nil disables
// the thinking block on the wire — preserves legacy behavior.
type ThinkingConfig struct {
	Enabled bool // send thinking{type:"enabled"} when true
	Budget  int  // optional budget_tokens cap; 0 = provider default / unbounded
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func NewProviderCompletionWithCache(c *llm.Client, model string, maxTokens int, temperature float64, cache *llm.PromptCache) func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
	return NewProviderCompletionFull(c, model, maxTokens, temperature, cache, nil)
}

// NewProviderCompletionFull is the canonical factory that wires every
// optional knob the wireRequest supports — cache + thinking. Nil cache
// or nil thinking preserves the legacy behavior of the simpler
// constructors above.
func NewProviderCompletionFull(c *llm.Client, model string, maxTokens int, temperature float64, cache *llm.PromptCache, thinking *ThinkingConfig) func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
	return func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		wt := make([]wireTool, 0, len(tools))
		for _, t := range tools {
			wt = append(wt, wireTool{Type: "function", Function: wireFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			}})
		}
		var wireThinkingField *wireThinking
		if thinking != nil && thinking.Enabled {
			wireThinkingField = &wireThinking{Type: "enabled"}
			if thinking.Budget > 0 {
				wireThinkingField.Budget = thinking.Budget
			}
		}
		body, err := json.Marshal(wireRequest{
			Model: model, Messages: history, Tools: wt,
			MaxTokens: maxTokens, Temperature: temperature,
			Thinking: wireThinkingField,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal completion request: %w", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if c.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
		}
		var cacheKey string
		var cacheHit bool
		if cache != nil && llm.SupportsCaching(model) {
			var systemPrompt, firstUser string
			for _, m := range history {
				if m.Role == "system" && systemPrompt == "" {
					systemPrompt = m.Content
				}
				if m.Role == "user" && firstUser == "" {
					firstUser = m.Content
				}
			}
			cacheKey = llm.CacheKey(systemPrompt, firstUser)
			httpReq.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
			if prefixID, ok := cache.Get(cacheKey); ok {
				cacheHit = true
				httpReq.Header.Set("X-SIN-Cache-Prefix-ID", prefixID)
				fmt.Fprintf(os.Stderr, "sin-code: prompt cache HIT (key=%s)\n", cacheKey[:12])
			} else {
				fmt.Fprintf(os.Stderr, "sin-code: prompt cache MISS (key=%s)\n", cacheKey[:12])
			}
			httpReq.Header.Set("X-SIN-Cache-Key", cacheKey)
		}
		resp, err := c.HTTP.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("LLM API error %d: %s", resp.StatusCode, string(data))
		}
		var out wireResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, fmt.Errorf("decode completion response: %w", err)
		}
		if len(out.Choices) == 0 {
			return nil, fmt.Errorf("LLM returned no choices")
		}
		if cache != nil && cacheKey != "" {
			if prefixID := resp.Header.Get("X-SIN-Cache-Prefix-ID"); prefixID != "" {
				cache.Set(cacheKey, prefixID)
				if !cacheHit {
					fmt.Fprintf(os.Stderr, "sin-code: prompt cache stored prefix (key=%s)\n", cacheKey[:12])
				}
			} else if out.ID != "" {
				cache.Set(cacheKey, out.ID)
				if !cacheHit {
					fmt.Fprintf(os.Stderr, "sin-code: prompt cache stored id (key=%s)\n", cacheKey[:12])
				}
			}
		}
		msg := out.Choices[0].Message

		raw := session.Message{Role: msg.Role, Content: msg.Content}
		if raw.Role == "" {
			raw.Role = "assistant"
		}
		if len(msg.ToolCalls) > 0 {
			rawTC, err := marshalToolCalls(msg.ToolCalls)
			if err != nil {
				return nil, fmt.Errorf("re-marshal tool_calls: %w", err)
			}
			raw.ToolCalls = rawTC
		}

		calls := make([]ToolCall, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			args := map[string]any{}
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					return nil, fmt.Errorf("tool call %s: bad arguments JSON: %w", tc.Function.Name, err)
				}
			}
			calls = append(calls, ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: args})
		}
		return &Completion{Text: msg.Content, ToolCalls: calls, Raw: raw, Usage: Usage{
			PromptTokens:     out.Usage.PromptTokens,
			CompletionTokens: out.Usage.CompletionTokens,
			TotalTokens:      out.Usage.TotalTokens,
			ThinkingTokens:   out.Usage.ThinkingTokens,
		}}, nil
	}
}

// --- SSE streaming structs (for NewProviderCompletionStream) ---------------

type sseToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type sseDelta struct {
	Role      string             `json:"role,omitempty"`
	Content   string             `json:"content,omitempty"`
	ToolCalls []sseToolCallDelta `json:"tool_calls,omitempty"`
}

type sseChoice struct {
	Index        int       `json:"index"`
	Delta        sseDelta  `json:"delta"`
	FinishReason string    `json:"finish_reason"`
}

type sseChunkResponse struct {
	ID      string      `json:"id"`
	Model   string      `json:"model"`
	Choices []sseChoice `json:"choices"`
	Usage   *wireUsage  `json:"usage,omitempty"`
}

// accumulatedToolCall collects incremental tool-call fragments across
// SSE chunks. OpenAI streaming sends the id+name in the first chunk for
// a given index and the arguments string in subsequent chunks.
type accumulatedToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

// NewProviderCompletionStream returns a Completion func that uses SSE
// streaming (stream=true) instead of a single-shot HTTP POST. Each
// content delta is forwarded to streamCallback in real time so the
// caller can print tokens as they arrive. Tool-call deltas are
// accumulated across chunks and reassembled into the returned
// Completion, making this a drop-in replacement for
// NewProviderCompletionFull.
func NewProviderCompletionStream(c *llm.Client, model string, maxTokens int, temperature float64, cache *llm.PromptCache, thinking *ThinkingConfig, streamCallback func(string)) func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
	if streamCallback == nil {
		streamCallback = func(string) {}
	}
	return func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		wt := make([]wireTool, 0, len(tools))
		for _, t := range tools {
			wt = append(wt, wireTool{Type: "function", Function: wireFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			}})
		}
		var wireThinkingField *wireThinking
		if thinking != nil && thinking.Enabled {
			wireThinkingField = &wireThinking{Type: "enabled"}
			if thinking.Budget > 0 {
				wireThinkingField.Budget = thinking.Budget
			}
		}
		body, err := json.Marshal(wireRequest{
			Model: model, Messages: history, Tools: wt,
			MaxTokens: maxTokens, Temperature: temperature,
			Thinking: wireThinkingField, Stream: true,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal stream request: %w", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.BaseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if c.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
		}
		var cacheKey string
		if cache != nil && llm.SupportsCaching(model) {
			var systemPrompt, firstUser string
			for _, m := range history {
				if m.Role == "system" && systemPrompt == "" {
					systemPrompt = m.Content
				}
				if m.Role == "user" && firstUser == "" {
					firstUser = m.Content
				}
			}
			cacheKey = llm.CacheKey(systemPrompt, firstUser)
			httpReq.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
			httpReq.Header.Set("X-SIN-Cache-Key", cacheKey)
		}
		resp, err := c.HTTP.Do(httpReq)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			data, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("LLM stream API error %d: %s", resp.StatusCode, string(data))
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

		var content strings.Builder
		var toolCalls map[int]*accumulatedToolCall
		var lastUsage *wireUsage
		var lastFinish string
		respID := ""
		respModel := model

		for scanner.Scan() {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "[DONE]" {
				break
			}
			if payload == "" {
				continue
			}
			var chunk sseChunkResponse
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				continue
			}
			if chunk.ID != "" {
				respID = chunk.ID
			}
			if chunk.Model != "" {
				respModel = chunk.Model
			}
			if chunk.Usage != nil {
				lastUsage = chunk.Usage
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			ch := chunk.Choices[0]
			if ch.FinishReason != "" {
				lastFinish = ch.FinishReason
			}
			if ch.Delta.Content != "" {
				content.WriteString(ch.Delta.Content)
				streamCallback(ch.Delta.Content)
			}
			for _, tcd := range ch.Delta.ToolCalls {
				if toolCalls == nil {
					toolCalls = make(map[int]*accumulatedToolCall)
				}
				tc, ok := toolCalls[tcd.Index]
				if !ok {
					tc = &accumulatedToolCall{}
					toolCalls[tcd.Index] = tc
				}
				if tcd.ID != "" {
					tc.id = tcd.ID
				}
				if tcd.Function.Name != "" {
					tc.name = tcd.Function.Name
				}
				if tcd.Function.Arguments != "" {
					tc.arguments.WriteString(tcd.Function.Arguments)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read SSE stream: %w", err)
		}

		text := content.String()
		raw := session.Message{Role: "assistant", Content: text}
		var calls []ToolCall
		if len(toolCalls) > 0 {
			// Build ordered list by index.
			indices := make([]int, 0, len(toolCalls))
			for idx := range toolCalls {
				indices = append(indices, idx)
			}
			sort.Ints(indices)
			var wireTCs []wireToolCall
			for _, idx := range indices {
				tc := toolCalls[idx]
				args := map[string]any{}
				if tc.arguments.Len() > 0 {
					if err := json.Unmarshal([]byte(tc.arguments.String()), &args); err != nil {
						return nil, fmt.Errorf("tool call %s: bad arguments JSON: %w", tc.name, err)
					}
				}
				calls = append(calls, ToolCall{ID: tc.id, Name: tc.name, Args: args})
				wireTCs = append(wireTCs, wireToolCall{
					ID:   tc.id,
					Type: "function",
				})
				wireTCs[len(wireTCs)-1].Function.Name = tc.name
				wireTCs[len(wireTCs)-1].Function.Arguments = tc.arguments.String()
			}
			if len(wireTCs) > 0 {
				rawTC, err := marshalToolCalls(wireTCs)
				if err != nil {
					return nil, fmt.Errorf("re-marshal tool_calls: %w", err)
				}
				raw.ToolCalls = rawTC
			}
		}

		_ = respID
		_ = respModel
		_ = lastFinish

		usage := Usage{}
		if lastUsage != nil {
			usage.PromptTokens = lastUsage.PromptTokens
			usage.CompletionTokens = lastUsage.CompletionTokens
			usage.TotalTokens = lastUsage.TotalTokens
			usage.ThinkingTokens = lastUsage.ThinkingTokens
		}
		return &Completion{Text: text, ToolCalls: calls, Raw: raw, Usage: usage}, nil
	}
}
