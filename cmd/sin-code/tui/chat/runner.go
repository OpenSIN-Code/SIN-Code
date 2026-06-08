// SPDX-License-Identifier: MIT
// Purpose: async LLM runner for the TUI chat view. Translates the
// `chat.SubmitMsg` payload (system + last 5 history turns + current prompt)
// into an `llm.ChatRequest` and returns the assistant text. Provider is
// resolved via `llm.ProviderFromConfig("nim", ...)`, which reads
// `SIN_NIM_API_KEY` from the environment.
package chat

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/llm"
)

const (
	defaultModel  = "meta/llama-3.3-70b-instruct"
	defaultSystem = "You are sin-code, an AI coding assistant. Be concise."
	historyKeepN  = 5
)

// Runner wraps an LLM client + model + system prompt. One Runner per TUI
// session; the chat view holds a singleton and reuses it across submits.
type Runner struct {
	Client       *llm.Client
	Model        string
	SystemPrompt string
}

// NewRunner builds a Runner from the current environment. Returns an
// error if no API key is configured (env SIN_NIM_API_KEY).
func NewRunner() (*Runner, error) {
	if os.Getenv("SIN_NIM_API_KEY") == "" {
		return nil, fmt.Errorf("no API key configured (set SIN_NIM_API_KEY)")
	}
	c, err := llm.ProviderFromConfig("nim", "", "", defaultModel, 0)
	if err != nil {
		return nil, err
	}
	return &Runner{
		Client:       c,
		Model:        defaultModel,
		SystemPrompt: defaultSystem,
	}, nil
}

// NewRunnerWithClient builds a Runner from a pre-configured client.
// Used by tests and by callers that want to override the base URL.
func NewRunnerWithClient(c *llm.Client, model, system string) *Runner {
	if model == "" {
		model = defaultModel
	}
	if system == "" {
		system = defaultSystem
	}
	return &Runner{Client: c, Model: model, SystemPrompt: system}
}

// Run builds a ChatRequest from system + last 5 history entries + current
// prompt and returns the assistant text. history is the full m.ChatHistory
// slice; the most recent `historyKeepN` entries are folded in.
func (r *Runner) Run(ctx context.Context, prompt string, history []string) (string, error) {
	if r == nil || r.Client == nil {
		return "", fmt.Errorf("runner not initialized")
	}
	model := r.Model
	if model == "" {
		model = defaultModel
	}
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

	req := llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   1024,
		Temperature: 0,
	}
	resp, err := r.Client.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.ExtractText(), nil
}

// ChatResponseMsg is fired by the background goroutine when the LLM
// returns. The TUI Update handler appends it to ChatHistory.
type ChatResponseMsg struct {
	Text  string
	Error error
}

// SendChatResponse is a tea.Cmd that produces a ChatResponseMsg. Useful
// for callers that drive the chat synchronously (e.g. tests). The chat
// input normally spawns a goroutine and uses *tea.Program.Send to push
// ChatResponseMsg into the event loop directly.
func SendChatResponse(text string, err error) tea.Cmd {
	return func() tea.Msg {
		return ChatResponseMsg{Text: text, Error: err}
	}
}
