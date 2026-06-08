// SPDX-License-Identifier: MIT
// Purpose: LLM provider definitions for NIM, OpenAI, Anthropic, Ollama, Groq,
// and any OpenAI-compatible endpoint. Each Provider has a base URL, a default
// model, and an env-var name for the API key. Agents reference providers
// by name in their agent.toml.
package llm

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Provider struct {
	Name        string
	BaseURL     string
	APIKeyEnv   string
	DefaultModel string
	Description string
}

var Providers = map[string]Provider{
	"nim": {
		Name:         "nim",
		BaseURL:      "https://integrate.api.nvidia.com/v1",
		APIKeyEnv:    "SIN_NIM_API_KEY",
		DefaultModel: NIMDefaultModel,
		Description: "NVIDIA NIM — cloud-hosted open models (Llama, Qwen, Kimi, etc.)",
	},
	"openai": {
		Name:         "openai",
		BaseURL:      "https://api.openai.com/v1",
		APIKeyEnv:    "OPENAI_API_KEY",
		DefaultModel: "gpt-4o",
		Description:  "OpenAI — GPT-4o, o1, etc.",
	},
	"anthropic": {
		Name:         "anthropic",
		BaseURL:      "https://api.anthropic.com/v1",
		APIKeyEnv:    "ANTHROPIC_API_KEY",
		DefaultModel: "claude-sonnet-4-5",
		Description:  "Anthropic — Claude (via OpenAI-compatible proxy or direct)",
	},
	"ollama": {
		Name:         "ollama",
		BaseURL:      "http://127.0.0.1:11434/v1",
		APIKeyEnv:    "",
		DefaultModel: "llama3.1",
		Description:  "Ollama — local models (no API key required)",
	},
	"groq": {
		Name:         "groq",
		BaseURL:      "https://api.groq.com/openai/v1",
		APIKeyEnv:    "GROQ_API_KEY",
		DefaultModel: "llama-3.3-70b-versatile",
		Description:  "Groq — fast inference for open models",
	},
	"custom": {
		Name:         "custom",
		BaseURL:      "",
		APIKeyEnv:    "",
		DefaultModel: "",
		Description:  "Custom OpenAI-compatible endpoint (set SIN_LLM_BASE_URL and SIN_LLM_API_KEY)",
	},
}

// LookupProvider returns the provider for a name, or an error if unknown.
func LookupProvider(name string) (Provider, error) {
	if p, ok := Providers[strings.ToLower(name)]; ok {
		return p, nil
	}
	return Provider{}, fmt.Errorf("unknown provider: %s (use: nim, openai, anthropic, ollama, groq, custom)", name)
}

// ProviderFromConfig resolves a provider by name with optional overrides:
//   - baseURL override
//   - apiKey override
//   - model override
// Returns a Client ready to chat.
func ProviderFromConfig(name, baseURLOverride, apiKeyOverride, modelOverride string, timeout time.Duration) (*Client, error) {
	prov, err := LookupProvider(name)
	if err != nil {
		return nil, err
	}
	baseURL := prov.BaseURL
	if baseURLOverride != "" {
		baseURL = baseURLOverride
	}
	if baseURL == "" {
		envBase := os.Getenv("SIN_LLM_BASE_URL")
		if envBase != "" {
			baseURL = envBase
		}
	}
	if baseURL == "" {
		return nil, fmt.Errorf("provider %s has no base URL; set base_url in agent.toml or SIN_LLM_BASE_URL", name)
	}
	apiKey := apiKeyOverride
	if apiKey == "" && prov.APIKeyEnv != "" {
		apiKey = os.Getenv(prov.APIKeyEnv)
	}
	if apiKey == "" {
		apiKey = os.Getenv("SIN_LLM_API_KEY")
	}
	if apiKey == "" && name != "ollama" {
		return nil, fmt.Errorf("provider %s requires API key; set %s or SIN_LLM_API_KEY", name, prov.APIKeyEnv)
	}
	if modelOverride == "" {
		modelOverride = prov.DefaultModel
	}
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: timeout},
	}, nil
}

// ListProviderNames returns all provider names.
func ListProviderNames() []string {
	out := make([]string, 0, len(Providers))
	for k := range Providers {
		out = append(out, k)
	}
	return out
}
