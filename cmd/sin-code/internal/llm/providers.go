// SPDX-License-Identifier: MIT
// Purpose: LLM provider definitions for NIM, OpenAI, Anthropic, Ollama, Groq,
// and any OpenAI-compatible endpoint. Each Provider has a base URL, a default
// model, and an env-var name for the API key. Agents reference providers
// by name in their agent.toml.
package llm

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Provider struct {
	Name         string
	BaseURL      string
	APIKeyEnv    string
	DefaultModel string
	Description  string
}

const (
	ClaudeFable5Model   = "claude-fable-5"
	ClaudeMythos5Model  = "claude-mythos-5"
	ClaudeFable5Context = 200_000
	ClaudeMythos5Context = 500_000
	ClaudeFable5MaxOut  = 128_000
	ClaudeMythos5MaxOut = 128_000
)

type ModelInfo struct {
	Name         string
	Provider     string
	MaxContext   int
	MaxOutput    int
	InputPer1M   float64
	OutputPer1M  float64
	RequiresThinking bool
}

var ModelRegistry = map[string]ModelInfo{
	ClaudeFable5Model: {
		Name:             ClaudeFable5Model,
		Provider:         "fable",
		MaxContext:       ClaudeFable5Context,
		MaxOutput:        ClaudeFable5MaxOut,
		InputPer1M:       10.0,
		OutputPer1M:      50.0,
		RequiresThinking: true,
	},
	ClaudeMythos5Model: {
		Name:             ClaudeMythos5Model,
		Provider:         "mythos",
		MaxContext:       ClaudeMythos5Context,
		MaxOutput:        ClaudeMythos5MaxOut,
		InputPer1M:       10.0,
		OutputPer1M:      50.0,
		RequiresThinking: true,
	},
}

func LookupModel(name string) (ModelInfo, bool) {
	info, ok := ModelRegistry[name]
	return info, ok
}

var Providers = map[string]Provider{
	"nim": {
		Name:         "nim",
		BaseURL:      "https://integrate.api.nvidia.com/v1",
		APIKeyEnv:    "SIN_NIM_API_KEY",
		DefaultModel: NIMDefaultModel,
		Description:  "NVIDIA NIM — cloud-hosted open models (Llama, Qwen, Kimi, etc.)",
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
	"fable": {
		Name:         "fable",
		BaseURL:      "https://api.anthropic.com/v1",
		APIKeyEnv:    "ANTHROPIC_API_KEY",
		DefaultModel: ClaudeFable5Model,
		Description:  "Anthropic Claude Fable 5 — strongest coding model (80.3% SWE-bench Pro)",
	},
	"mythos": {
		Name:         "mythos",
		BaseURL:      "https://api.anthropic.com/v1",
		APIKeyEnv:    "ANTHROPIC_API_KEY",
		DefaultModel: ClaudeMythos5Model,
		Description:  "Anthropic Claude Mythos 5 — unrestricted-access variant of Fable 5",
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
//
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
	// Build the client through NewClient so the breaker wiring stays
	// in one place. NewClient takes (baseURL, apiKey) and constructs
	// its own *http.Client with a wrapped transport; we then patch
	// the timeout back in to honor the caller's choice.
	c := NewClient(baseURL, apiKey)
	c.HTTP.Timeout = timeout
	return c, nil
}

// ListProviderNames returns all provider names.
func ListProviderNames() []string {
	out := make([]string, 0, len(Providers))
	for k := range Providers {
		out = append(out, k)
	}
	return out
}
