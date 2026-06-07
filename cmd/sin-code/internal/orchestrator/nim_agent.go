// SPDX-License-Identifier: MIT
// Purpose: NIMAgent — Agent implementation backed by NVIDIA NIM (any
// OpenAI-compatible chat/completions endpoint). Reads its system prompt
// from AgentConfig.SystemFile (searched on disk), pastes prior scratchpad
// context into the user prompt, calls the LLM, and writes outputs + token
// usage back to the scratchpad. Env vars: SIN_NIM_API_KEY, SIN_NIM_BASE_URL,
// SIN_AGENTS_DIR.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/llm"
)

type NIMAgent struct {
	cfg    AgentConfig
	client *llm.Client
}

func NewNIMAgent(cfg AgentConfig) *NIMAgent {
	return NewNIMAgentWithClient(cfg, nil)
}

func NewNIMAgentWithClient(cfg AgentConfig, client *llm.Client) *NIMAgent {
	if client == nil {
		baseURL := os.Getenv("SIN_NIM_BASE_URL")
		if baseURL == "" {
			baseURL = llm.NIMDefaultBaseURL
		}
		apiKey := os.Getenv("SIN_NIM_API_KEY")
		client = llm.NewClient(baseURL, apiKey)
	}
	return &NIMAgent{cfg: cfg, client: client}
}

func (n *NIMAgent) Name() string        { return n.cfg.Name }
func (n *NIMAgent) Config() AgentConfig { return n.cfg }

func (n *NIMAgent) Run(ctx context.Context, task *Task, scratch *Scratchpad) (string, error) {
	systemPrompt, err := n.loadSystemPrompt()
	if err != nil {
		return "", fmt.Errorf("load system prompt: %w", err)
	}

	priorInputs, _ := scratch.Read("inputs")
	var priorOutputs []string
	for k, v := range scratch.ReadAll() {
		if strings.HasPrefix(k, "outputs:") {
			priorOutputs = append(priorOutputs, fmt.Sprintf("[%s]\n%s", k, v.Content))
		}
	}

	userPrompt := n.buildUserPrompt(task, priorInputs, priorOutputs)
	scratch.Write(n.cfg.Name, "inputs", task.Description)

	req := llm.ChatRequest{
		Model:       llm.ResolveModel(n.cfg.Model),
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   n.cfg.MaxTokens,
		Temperature: n.cfg.Temperature,
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	resp, err := n.client.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	out := resp.ExtractText()
	scratch.Write(n.cfg.Name, "outputs:"+task.ID, out)
	scratch.Write(n.cfg.Name, "usage:"+task.ID, fmt.Sprintf("prompt=%d completion=%d total=%d",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens))

	return out, nil
}

func (n *NIMAgent) loadSystemPrompt() (string, error) {
	if n.cfg.SystemFile == "" {
		return n.defaultSystemPrompt(), nil
	}
	candidates := []string{
		n.cfg.SystemFile,
		filepath.Join(".", n.cfg.SystemFile),
		filepath.Join(os.Getenv("HOME"), ".config", "sin-code", n.cfg.SystemFile),
	}
	if env := os.Getenv("SIN_AGENTS_DIR"); env != "" {
		candidates = append(candidates, filepath.Join(env, n.cfg.SystemFile))
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if data, err := os.ReadFile(p); err == nil {
			return string(data), nil
		}
	}
	return n.defaultSystemPrompt(), nil
}

func (n *NIMAgent) defaultSystemPrompt() string {
	return fmt.Sprintf("You are %s, a specialized agent.\n\nType: %s\nDescription: %s\n\nRespond concisely and accurately. Use the available scratchpad context to inform your answer.",
		n.cfg.Name, n.cfg.Type, n.cfg.Description)
}

func (n *NIMAgent) buildUserPrompt(task *Task, priorInputs string, priorOutputs []string) string {
	var b strings.Builder
	b.WriteString("## Task\n")
	b.WriteString(fmt.Sprintf("ID: %s\n", task.ID))
	b.WriteString(fmt.Sprintf("Type: %s\n", task.Type))
	b.WriteString(fmt.Sprintf("Description: %s\n", task.Description))
	if task.AgentName != "" {
		b.WriteString(fmt.Sprintf("Assigned Agent: %s\n", task.AgentName))
	}
	if priorInputs != "" {
		b.WriteString("\n## Prior Context (from scratchpad)\n")
		b.WriteString(priorInputs)
	}
	if len(priorOutputs) > 0 {
		b.WriteString("\n## Prior Outputs (from scratchpad)\n")
		b.WriteString(strings.Join(priorOutputs, "\n\n"))
	}
	return b.String()
}
