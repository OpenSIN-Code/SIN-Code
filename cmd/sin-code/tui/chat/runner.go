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
		return "", 0, err
	}
	return resp.ExtractText(), resp.Usage.TotalTokens, nil
}

func (r *Runner) RunStream(ctx context.Context, prompt string, history []string, onChunk func(string)) (string, int, error) {
	if r == nil || r.Client == nil {
		return "", 0, fmt.Errorf("runner not initialized")
	}
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
