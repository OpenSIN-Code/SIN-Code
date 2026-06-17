// SPDX-License-Identifier: MIT
package chat

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

var (
	providerFromConfigHook = llm.ProviderFromConfig
)

const (
	defaultModel  = "meta/llama-3.3-70b-instruct"
	defaultSystem = "You are sin-code, an AI coding assistant. Be concise."
	historyKeepN  = 5
)

type Runner struct {
	Client       *llm.Client
	Model        string
	SystemPrompt string

	// StreamMode records which streaming path the most recent
	// RunStream call took: "real" for SSE, "fake" for the
	// word-by-word fallback, "" before any streaming call. The
	// TUI can query this to surface an honest indicator to the
	// user instead of presenting simulated streaming as live SSE.
	StreamMode string
}

func NewRunner() (*Runner, error) {
	if os.Getenv("SIN_NIM_API_KEY") == "" {
		return nil, fmt.Errorf("no API key configured (set SIN_NIM_API_KEY)")
	}
	model := os.Getenv("SIN_LLM_MODEL")
	if model == "" {
		model = defaultModel
	}
	c, err := providerFromConfigHook("nim", "", "", model, 0)
	if err != nil {
		return nil, err
	}
	return &Runner{
		Client:       c,
		Model:        model,
		SystemPrompt: defaultSystem,
	}, nil
}

func NewRunnerWithClient(c *llm.Client, model, system string) *Runner {
	if model == "" {
		model = defaultModel
	}
	if system == "" {
		system = defaultSystem
	}
	return &Runner{Client: c, Model: model, SystemPrompt: system}
}

func (r *Runner) Run(ctx context.Context, prompt string, history []string) (string, int, error) {
	if r == nil || r.Client == nil {
		return "", 0, fmt.Errorf("runner not initialized")
	}
	model := r.Model
	if model == "" {
		model = defaultModel
	}
	messages := r.buildMessages(prompt, history)

	req := llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   1024,
		Temperature: 0,
	}
	resp, err := r.Client.Chat(ctx, req)
	if err != nil {
		return "", 0, err
	}
	return resp.ExtractText(), resp.Usage.TotalTokens, nil
}

// buildMessages assembles the message slice from the system prompt,
// trimmed recent history (last historyKeepN entries), and the current
// user prompt. History entries may be prefixed with "user:" or
// "assistant:" to set the role; unprefixed entries default to "user".
// Empty entries (after prefix stripping) are skipped.
func (r *Runner) buildMessages(prompt string, history []string) []llm.Message {
	system := r.SystemPrompt
	if system == "" {
		system = defaultSystem
	}
	messages := []llm.Message{
		{Role: "system", Content: system},
	}
	if n := len(history); n > 0 {
		start := n - historyKeepN
		if start < 0 {
			start = 0
		}
		for _, entry := range history[start:] {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			role := "user"
			switch {
			case strings.HasPrefix(entry, "assistant:"):
				role = "assistant"
				entry = strings.TrimSpace(strings.TrimPrefix(entry, "assistant:"))
			case strings.HasPrefix(entry, "user:"):
				entry = strings.TrimSpace(strings.TrimPrefix(entry, "user:"))
			}
			if entry == "" {
				continue
			}
			messages = append(messages, llm.Message{Role: role, Content: entry})
		}
	}
	messages = append(messages, llm.Message{Role: "user", Content: prompt})
	return messages
}

func (r *Runner) RunStream(ctx context.Context, prompt string, history []string, onChunk func(string)) (string, int, error) {
	if r == nil || r.Client == nil {
		return "", 0, fmt.Errorf("runner not initialized")
	}
	if onChunk == nil {
		onChunk = func(string) {}
	}

	// Real SSE streaming path — available on every Client with an
	// HTTP transport. Falls back to fake word-by-word streaming when
	// the provider rejects stream=true or the connection fails
	// mid-stream, so the TUI never shows a blank response.
	if r.Client.HasStreaming() {
		r.StreamMode = "real"
		text, tokens, err := r.runRealStream(ctx, prompt, history, onChunk)
		if err == nil {
			return text, tokens, nil
		}
		// Streaming failed — fall through to the non-streaming
		// fallback so the user still gets a complete answer.
	}

	// Fake streaming fallback. Emit a single-line notice via
	// onChunk so the user can distinguish simulated streaming from
	// real SSE. The notice is visual only — it is NOT included in
	// the returned text, so it does not pollute the recorded
	// response (the TUI overwrites the streamed buffer with the
	// final text on completion).
	r.StreamMode = "fake"
	onChunk("⚠ streaming simulated — API does not support SSE\n\n")
	return r.runFakeStream(ctx, prompt, history, onChunk)
}

// runRealStream opens an SSE connection and forwards each delta
// content fragment to onChunk as it arrives.
func (r *Runner) runRealStream(ctx context.Context, prompt string, history []string, onChunk func(string)) (string, int, error) {
	model := r.Model
	if model == "" {
		model = defaultModel
	}
	messages := r.buildMessages(prompt, history)

	req := llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   1024,
		Temperature: 0,
	}

	var fullText strings.Builder
	resp, err := r.Client.ChatStream(ctx, req, func(chunk llm.StreamChunk) {
		if chunk.Error != nil || chunk.Done {
			return
		}
		if chunk.Content != "" {
			fullText.WriteString(chunk.Content)
			onChunk(chunk.Content)
		}
	})
	if err != nil {
		return fullText.String(), 0, err
	}

	tokens := 0
	if resp != nil {
		tokens = resp.Usage.TotalTokens
		// If the provider did not send usage in the stream, fall
		// back to the accumulated text from the response object.
		if fullText.Len() == 0 && resp.ExtractText() != "" {
			return resp.ExtractText(), tokens, nil
		}
	}
	return fullText.String(), tokens, nil
}

// runFakeStream is the legacy fallback: a single non-streaming Chat
// call whose result is dribbled out word-by-word with a small delay.
// Used when the provider does not support SSE or the streaming
// request failed.
func (r *Runner) runFakeStream(ctx context.Context, prompt string, history []string, onChunk func(string)) (string, int, error) {
	full, tokens, err := r.Run(ctx, prompt, history)
	if err != nil {
		return "", 0, err
	}
	words := strings.Fields(full)
	var sb strings.Builder
	for i, w := range words {
		if i > 0 {
			sb.WriteString(" ")
			onChunk(" ")
		}
		sb.WriteString(w)
		onChunk(w)
		select {
		case <-time.After(8 * time.Millisecond):
		case <-ctx.Done():
			return sb.String(), 0, ctx.Err()
		}
	}
	return sb.String(), tokens, nil
}

type ChatChunkMsg struct {
	Text string
}

type ChatResponseMsg struct {
	Text   string
	Error  error
	Tokens int
}

func SendChatResponse(text string, err error) tea.Cmd {
	return func() tea.Msg {
		return ChatResponseMsg{Text: text, Error: err}
	}
}
