// SPDX-License-Identifier: MIT
// Purpose: LLMAgent — provider-agnostic LLM-backed agent. Uses the Provider
// field in AgentConfig to pick a backend (nim, openai, anthropic, ollama,
// groq, custom). Backwards-compatible: NIMAgent is a thin wrapper.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/llm"
)

type LLMAgent struct {
	cfg    AgentConfig
	client *llm.Client
}

func NewLLMAgent(cfg AgentConfig) *LLMAgent {
	return NewLLMAgentWithClient(cfg, nil)
}

func NewLLMAgentWithClient(cfg AgentConfig, client *llm.Client) *LLMAgent {
	if client != nil {
		return &LLMAgent{cfg: cfg, client: client}
	}
	providerName := cfg.Provider
	if providerName == "" {
		providerName = inferProviderFromEnv()
	}
	if providerName == "" {
		providerName = "nim"
	}
	c, err := llm.ProviderFromConfig(providerName, cfg.BaseURL, "", cfg.Model, 0)
	if err != nil {
		return &LLMAgent{cfg: cfg, client: nil}
	}
	return &LLMAgent{cfg: cfg, client: c}
}

func inferProviderFromEnv() string {
	if os.Getenv("SIN_NIM_API_KEY") != "" {
		return "nim"
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return "openai"
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "anthropic"
	}
	if os.Getenv("GROQ_API_KEY") != "" {
		return "groq"
	}
	return ""
}

// Backwards-compat alias.
type NIMAgent = LLMAgent

func NewNIMAgent(cfg AgentConfig) *NIMAgent { return NewLLMAgent(cfg) }
func NewNIMAgentWithClient(cfg AgentConfig, client *llm.Client) *NIMAgent {
	return NewLLMAgentWithClient(cfg, client)
}

func (a *LLMAgent) Name() string        { return a.cfg.Name }
func (a *LLMAgent) Config() AgentConfig { return a.cfg }

func (a *LLMAgent) Run(ctx context.Context, task *Task, scratch *Scratchpad) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("agent %s: no LLM client (missing API key?)", a.cfg.Name)
	}
	systemPrompt, err := a.loadSystemPrompt()
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

	userPrompt := a.buildUserPrompt(task, priorInputs, priorOutputs)
	scratch.Write(a.cfg.Name, "inputs", task.Description)

	model := a.cfg.Model
	if a.cfg.Provider != "" || a.cfg.BaseURL != "" {
		if a.cfg.Provider == "" {
			a.cfg.Provider = inferProviderFromEnv()
		}
		if prov, perr := llm.LookupProvider(a.cfg.Provider); perr == nil && a.cfg.Model == "" {
			model = prov.DefaultModel
		}
	} else {
		model = llm.ResolveModel(a.cfg.Model)
	}
	if model == "" {
		prov, _ := llm.LookupProvider(a.cfg.Provider)
		model = prov.DefaultModel
	}

	req := llm.ChatRequest{
		Model:       model,
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   a.cfg.MaxTokens,
		Temperature: a.cfg.Temperature,
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}

	resp, err := a.client.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	out := resp.ExtractText()
	scratch.Write(a.cfg.Name, "outputs:"+task.ID, out)
	scratch.Write(a.cfg.Name, "usage:"+task.ID, fmt.Sprintf("prompt=%d completion=%d total=%d",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens))

	return out, nil
}

func (a *LLMAgent) loadSystemPrompt() (string, error) {
	if a.cfg.SystemFile == "" {
		return a.defaultSystemPrompt(), nil
	}
	candidates := []string{
		a.cfg.SystemFile,
		filepath.Join(".", a.cfg.SystemFile),
		filepath.Join(os.Getenv("HOME"), ".config", "sin-code", a.cfg.SystemFile),
	}
	if env := os.Getenv("SIN_AGENTS_DIR"); env != "" {
		candidates = append(candidates, filepath.Join(env, a.cfg.SystemFile))
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if data, err := os.ReadFile(p); err == nil {
			return string(data), nil
		}
	}
	return a.defaultSystemPrompt(), nil
}

func (a *LLMAgent) defaultSystemPrompt() string {
	return fmt.Sprintf("You are %s, a specialized agent.\n\nType: %s\nDescription: %s\n\nRespond concisely and accurately. Use the available scratchpad context to inform your answer.",
		a.cfg.Name, a.cfg.Type, a.cfg.Description)
}

func (a *LLMAgent) buildUserPrompt(task *Task, priorInputs string, priorOutputs []string) string {
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
