// SPDX-License-Identifier: MIT
// Purpose: bridges internal/llm.Client (STRUCT) to the agentloop.Completion
// func signature via a func-closure (issue #44). Adds OpenAI-compatible
// tool calling on top of the plain-chat client.
package agentloop

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

type wireRequest struct {
	Model       string            `json:"model"`
	Messages    []session.Message `json:"messages"`
	Tools       []wireTool        `json:"tools,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
}

type wireToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireResponse struct {
	Choices []struct {
		Message struct {
			Role      string         `json:"role"`
			Content   string         `json:"content"`
			ToolCalls []wireToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func NewProviderCompletion(c *llm.Client, model string, maxTokens int, temperature float64) func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
	return func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		wt := make([]wireTool, 0, len(tools))
		for _, t := range tools {
			wt = append(wt, wireTool{Type: "function", Function: wireFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			}})
		}
		body, err := json.Marshal(wireRequest{
			Model: model, Messages: history, Tools: wt,
			MaxTokens: maxTokens, Temperature: temperature,
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
		msg := out.Choices[0].Message

		raw := session.Message{Role: msg.Role, Content: msg.Content}
		if raw.Role == "" {
			raw.Role = "assistant"
		}
		if len(msg.ToolCalls) > 0 {
			rawTC, err := json.Marshal(msg.ToolCalls)
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
		return &Completion{Text: msg.Content, ToolCalls: calls, Raw: raw}, nil
	}
}
