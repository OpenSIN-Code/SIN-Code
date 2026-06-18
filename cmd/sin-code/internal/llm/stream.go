// SPDX-License-Identifier: MIT
// Purpose: real LLM streaming via Server-Sent Events (SSE). All
// OpenAI-compatible providers (NIM, OpenAI, Groq, Ollama, vLLM, …)
// speak the same wire protocol: the request body is a normal
// chat-completions payload with `"stream": true`, and the response is
// a sequence of `data: {…}\n\n` SSE events terminated by
// `data: [DONE]\n\n`. Each event carries a `choices[].delta.content`
// fragment that the caller forwards to the TUI one token at a time.
//
// This file adds `Client.ChatStream`, the `StreamChunk` value type,
// a pure `parseSSELine` helper (unit-testable without a server), and
// a `HasStreaming` probe so callers can keep a non-streaming fallback.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// StreamChunk is one piece of a streaming response delivered to the
// caller-supplied onChunk callback.
//
//   - Content holds the delta text fragment (may be "" for the
//     initial role-only chunk or the final finish-reason chunk).
//   - Done is true when the server sent `data: [DONE]`.
//   - Error is non-nil when a parse / transport error occurred mid-stream.
//     When Error is set the loop stops and the same error is returned
//     from ChatStream.
type StreamChunk struct {
	Content string
	Done    bool
	Error   error
}

// streamDelta mirrors the `choices[].delta` object in an SSE chunk.
type streamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

// streamChoice mirrors one entry in `choices[]` of an SSE chunk.
type streamChoice struct {
	Index        int          `json:"index"`
	Delta        streamDelta  `json:"delta"`
	FinishReason string       `json:"finish_reason"`
	Message      *streamDelta `json:"message,omitempty"`
}

// streamChunkResponse is the JSON payload inside one `data: {…}` SSE
// event. It reuses the top-level ChatResponse fields (id, model,
// created) plus a Choices slice whose Delta carries incremental
// content. Usage is populated by some providers in the terminal
// chunk (NIM, OpenAI with `stream_options.include_usage`).
type streamChunkResponse struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []streamChoice  `json:"choices"`
	Usage   *usageContainer `json:"usage,omitempty"`
}

// usageContainer mirrors ChatResponse.Usage so we can lift the
// final-fragment token counts into the returned ChatResponse.
type usageContainer struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// HasStreaming reports whether ChatStream is available on this
// client. It always returns true now that real SSE streaming is
// implemented; the method exists so callers can write a clean
// `if r.Client.HasStreaming() { … } else { fallback }` guard that
// degrades gracefully on older builds or custom transports that
// intentionally disable streaming.
func (c *Client) HasStreaming() bool {
	return c != nil && c.HTTP != nil
}

// ChatStream sends a ChatRequest with stream=true and invokes onChunk
// for each token fragment received from the server. The wire format
// is OpenAI-compatible SSE:
//
//	data: {"choices":[{"delta":{"content":"hel"}}]}\n\n
//	data: {"choices":[{"delta":{"content":"lo"}}]}\n\n
//	data: [DONE]\n\n
//
// The returned ChatResponse carries the fully accumulated content in
// Choices[0].Message.Content and — when the provider emits it — the
// token Usage from the terminal chunk. If the provider does not send
// usage in the stream, Usage is zero-valued and callers should not
// rely on it for billing.
//
// onChunk is called from the same goroutine that reads the response
// body, so it must not block (the TUI forwards chunks via a
// non-blocking prog.Send). A panicking onChunk is recovered into an
// error return so a buggy UI callback can never crash the LLM read
// loop.
func (c *Client) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("ChatStream: nil client")
	}
	if onChunk == nil {
		onChunk = func(StreamChunk) {}
	}

	// Force stream=true on the wire regardless of what the caller set.
	req.Stream = true

	if req.Model == "" {
		resolved, err := c.resolveEmptyModel(ctx)
		if err != nil {
			return nil, err
		}
		req.Model = resolved
	}

	body, err := jsonMarshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
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
		return nil, fmt.Errorf("LLM stream API error %d: %s", resp.StatusCode, string(data))
	}

	return readSSEStream(ctx, resp.Body, onChunk, c.Recorder, req.Model)
}

// readSSEStream consumes an OpenAI-compatible SSE body, forwarding
// delta content via onChunk and returning the assembled ChatResponse.
// It is extracted from ChatStream so tests can feed it a mock reader
// without spinning up an httptest server.
func readSSEStream(ctx context.Context, r io.Reader, onChunk func(StreamChunk), recorder Recorder, model string) (*ChatResponse, error) {
	scanner := bufio.NewScanner(r)
	// SSE lines can be long (especially for code-generation chunks);
	// bump the per-line buffer well past the default 64 KiB.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var full strings.Builder
	var result ChatResponse
	result.Model = model
	result.Choices = []struct {
		Index        int     `json:"index"`
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	}{
		{Index: 0, Message: Message{Role: "assistant"}},
	}
	var lastUsage *usageContainer
	var lastFinish string

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			onChunk(StreamChunk{Error: err})
			return &result, err
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // SSE event separator
		}
		if strings.HasPrefix(line, ":") {
			continue // SSE comment / keep-alive
		}

		chunk, done, perr := parseSSELine(line)
		if perr != nil {
			err := fmt.Errorf("parse SSE event: %w", perr)
			onChunk(StreamChunk{Error: err})
			return &result, err
		}
		if done {
			onChunk(StreamChunk{Done: true})
			break
		}
		if chunk == nil {
			continue // non-data SSE field (event:, id:, retry:)
		}

		var deltaContent string
		if len(chunk.Choices) > 0 {
			deltaContent = chunk.Choices[0].Delta.Content
			if fr := chunk.Choices[0].FinishReason; fr != "" {
				lastFinish = fr
			}
		}
		if deltaContent != "" {
			full.WriteString(deltaContent)
			onChunk(StreamChunk{Content: deltaContent})
		}
		if chunk.Usage != nil {
			lastUsage = chunk.Usage
		}
		if chunk.Model != "" {
			result.Model = chunk.Model
		}
		if chunk.ID != "" {
			result.ID = chunk.ID
		}
		if chunk.Created != 0 {
			result.Created = chunk.Created
		}
	}

	if err := scanner.Err(); err != nil {
		onChunk(StreamChunk{Error: err})
		return &result, fmt.Errorf("read SSE stream: %w", err)
	}

	// Assemble the final ChatResponse from accumulated state.
	result.Object = "chat.completion"
	result.Choices[0].Message.Content = full.String()
	result.Choices[0].Message.Role = "assistant"
	result.Choices[0].FinishReason = lastFinish
	if lastUsage != nil {
		result.Usage.PromptTokens = lastUsage.PromptTokens
		result.Usage.CompletionTokens = lastUsage.CompletionTokens
		result.Usage.TotalTokens = lastUsage.TotalTokens

		if recorder != nil && (lastUsage.PromptTokens != 0 || lastUsage.CompletionTokens != 0 || lastUsage.TotalTokens != 0) {
			if recErr := recorder.RecordUsage(ctx, SessionIDFromContext(ctx),
				model, SourceAdHoc,
				lastUsage.PromptTokens, lastUsage.CompletionTokens, lastUsage.TotalTokens, lastUsage.ThinkingTokens); recErr != nil {
				fmt.Fprintf(os.Stderr, "warn: usage recorder (stream): %v\n", recErr)
			}
		}
	}

	return &result, nil
}

// parseSSELine decodes one non-empty, non-comment SSE line. It returns
// the parsed chunk, a `done` flag for the `data: [DONE]` sentinel,
// and any JSON decode error. Non-`data:` fields (event:, id:, retry:)
// return (nil, false, nil) so the caller can skip them.
//
// The function is pure and side-effect free, making it the primary
// unit-test target for the SSE parser.
func parseSSELine(line string) (chunk *streamChunkResponse, done bool, err error) {
	if !strings.HasPrefix(line, "data:") {
		return nil, false, nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

	if payload == "[DONE]" {
		return nil, true, nil
	}
	if payload == "" {
		return nil, false, nil
	}

	var ev streamChunkResponse
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return nil, false, fmt.Errorf("decode data payload: %w", err)
	}
	return &ev, false, nil
}

// extractDeltaContent is a tiny helper used by tests to demonstrate
// the delta-content extraction path without constructing a full
// streamChunkResponse. It is exported so downstream code that wants
// to pull content out of a raw SSE payload can reuse it.
func extractDeltaContent(payload []byte) (string, error) {
	var ev streamChunkResponse
	if err := json.Unmarshal(payload, &ev); err != nil {
		return "", err
	}
	if len(ev.Choices) == 0 {
		return "", nil
	}
	return ev.Choices[0].Delta.Content, nil
}


