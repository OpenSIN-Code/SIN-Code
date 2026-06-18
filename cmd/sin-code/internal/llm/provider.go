// SPDX-License-Identifier: MIT
// Purpose: generic OpenAI-compatible LLM client. Single-shot chat completion
// request with bearer auth, JSON marshaling, and typed response decoding.
// Persists parsed tool-usage via a pluggable Recorder (issue #168).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/circuitbreaker"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client

	// breaker wraps the outbound transport so a misbehaving LLM
	// provider can no longer pin the agent loop on every chat call.
	// Constructed by NewClient / ProviderFromConfig; nil is tolerated
	// in tests that hand-build the struct (HTTP roundtrips still work,
	// just without breaker protection).
	breaker *circuitbreaker.Breaker

	// Recorder persists parsed ChatResponse.Usage on every successful
	// call. Defaults to NopRecorder (drained) when wired via NewClient.
	// Issue #168: previously the Usage block was parsed but dropped at
	// provider.go:42-46; it now flows through a pluggable recorder.
	Recorder Recorder
}

func NewClient(baseURL, apiKey string) *Client {
	br := circuitbreaker.New(&circuitbreaker.Config{
		Name:             "llm",
		FailureThreshold: 5,
		OpenDuration:     30 * time.Second,
		HalfOpenProbes:   1,
		SuccessThreshold: 1,
	})
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP: &http.Client{
			Timeout:   120 * time.Second,
			Transport: circuitbreaker.RoundTripper(http.DefaultTransport, br),
		},
		breaker:  br,
		Recorder: NopRecorder{},
	}
}

// BreakerStats exposes the breaker's snapshot for diagnostic surfaces
// (loopbuilder health, agentloop status). Returns nil if the client
// was constructed without going through NewClient / ProviderFromConfig.
func (c *Client) BreakerStats() *circuitbreaker.Stats {
	if c == nil || c.breaker == nil {
		return nil
	}
	s := c.breaker.Stats()
	return &s
}

// jsonMarshal is a test seam around json.Marshal.
var jsonMarshal = json.Marshal

func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	// Belt-and-suspenders: if the caller left Model empty, try to discover a
	// default. Order of preference: explicit env override -> /v1/models probe
	// (when we have an API key) -> error. Without this guard the request
	// would go out with model="" and NIM/OpenAI return 400.
	if req.Model == "" {
		resolved, err := c.resolveEmptyModel(ctx)
		if err != nil {
			return nil, err
		}
		req.Model = resolved
	}
	body, err := jsonMarshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
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
	var out ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	// Issue #168: persist parsed Usage. Best-effort — a recorder failure
	// never blocks the caller. SessionID comes from the context (set by
	// agentloop.Loop, the tui/chat runner, or the spec author).
	if c.Recorder != nil && (out.Usage.PromptTokens != 0 || out.Usage.CompletionTokens != 0 || out.Usage.TotalTokens != 0) {
		if recErr := c.Recorder.RecordUsage(ctx, SessionIDFromContext(ctx),
			req.Model, SourceAdHoc,
			out.Usage.PromptTokens, out.Usage.CompletionTokens, out.Usage.TotalTokens); recErr != nil {
			fmt.Fprintf(os.Stderr, "warn: usage recorder: %v\n", recErr)
		}
	}
	return &out, nil
}

func (r *ChatResponse) ExtractText() string {
	if len(r.Choices) > 0 {
		return r.Choices[0].Message.Content
	}
	return ""
}

func (c *Client) resolveEmptyModel(ctx context.Context) (string, error) {
	if v := os.Getenv("SIN_LLM_MODEL"); v != "" {
		return v, nil
	}
	if c.APIKey == "" {
		return "", fmt.Errorf("no model specified and no default available (set ChatRequest.Model or SIN_LLM_MODEL)")
	}
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/models", nil)
	if err != nil {
		return "", fmt.Errorf("build /v1/models request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("probe /v1/models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("probe /v1/models: status %d: %s", resp.StatusCode, string(data))
	}
	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return "", fmt.Errorf("decode /v1/models: %w", err)
	}
	if len(listing.Data) == 0 || listing.Data[0].ID == "" {
		return "", fmt.Errorf("no model specified and /v1/models returned no entries")
	}
	return listing.Data[0].ID, nil
}
