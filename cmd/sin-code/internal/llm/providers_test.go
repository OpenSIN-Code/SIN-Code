// SPDX-License-Identifier: MIT
// Purpose: tests for the LLM provider registry (nim/openai/anthropic/etc).
package llm

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestProvidersMap(t *testing.T) {
	required := []string{"nim", "openai", "anthropic", "ollama", "groq", "custom"}
	for _, name := range required {
		if _, ok := Providers[name]; !ok {
			t.Errorf("missing provider: %s", name)
		}
	}
}

func TestLookupProvider(t *testing.T) {
	for _, name := range []string{"nim", "NIM", "Ollama"} {
		p, err := LookupProvider(name)
		if err != nil {
			t.Errorf("lookup %s: %v", name, err)
		}
		if p.Name == "" {
			t.Errorf("empty name for %s", name)
		}
	}
}

func TestLookupProviderUnknown(t *testing.T) {
	_, err := LookupProvider("nope")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProviderFromConfigNIM(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "test-key")
	c, err := ProviderFromConfig("nim", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.BaseURL, "integrate.api.nvidia.com") {
		t.Errorf("base URL: %s", c.BaseURL)
	}
	if c.APIKey != "test-key" {
		t.Errorf("api key: %s", c.APIKey)
	}
}

func TestProviderFromConfigOpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	c, err := ProviderFromConfig("openai", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.BaseURL, "api.openai.com") {
		t.Errorf("base URL: %s", c.BaseURL)
	}
}

func TestProviderFromConfigOllamaNoKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("SIN_NIM_API_KEY", "")
	c, err := ProviderFromConfig("ollama", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "" {
		t.Errorf("ollama should not need key, got %s", c.APIKey)
	}
}

func TestProviderFromConfigMissingKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("SIN_NIM_API_KEY", "")
	t.Setenv("SIN_LLM_API_KEY", "")
	_, err := ProviderFromConfig("openai", "", "", "", 0)
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestProviderFromConfigOverrideBaseURL(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "test")
	c, err := ProviderFromConfig("nim", "https://my-proxy.example.com/v1", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://my-proxy.example.com/v1" {
		t.Errorf("override base URL: %s", c.BaseURL)
	}
}

func TestProviderFromConfigFallbackKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GROQ_API_KEY", "")
	t.Setenv("SIN_NIM_API_KEY", "")
	t.Setenv("SIN_LLM_API_KEY", "fallback-key")
	c, err := ProviderFromConfig("openai", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c.APIKey != "fallback-key" {
		t.Errorf("fallback key: %s", c.APIKey)
	}
}

func TestListProviderNames(t *testing.T) {
	names := ListProviderNames()
	if len(names) < 5 {
		t.Errorf("expected at least 5, got %d", len(names))
	}
}

func TestProviderDefaultModels(t *testing.T) {
	for name, p := range Providers {
		if name == "custom" {
			continue
		}
		if p.DefaultModel == "" {
			t.Errorf("provider %s has no default model", name)
		}
		if p.BaseURL == "" {
			t.Errorf("provider %s has no base URL", name)
		}
	}
}

func TestResolveModelPreservesUnknown(t *testing.T) {
	unknown := "vendor/custom-model-v9"
	if got := ResolveModel(unknown); got != unknown {
		t.Errorf("expected %s, got %s", unknown, got)
	}
}

func TestResolveModelAllAliases(t *testing.T) {
	for alias, expected := range NIMModelAliases {
		if got := ResolveModel(alias); got != expected {
			t.Errorf("alias %q: got %q, want %q", alias, got, expected)
		}
	}
}

func TestProviderFromConfigDefaultModelApplied(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "test")
	c, err := ProviderFromConfig("nim", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("nil client")
	}
}

func TestProviderFromConfigTimeout(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "test")
	c, _ := ProviderFromConfig("nim", "", "", "", 5*time.Second)
	if c.HTTP.Timeout.Seconds() != 5 {
		t.Errorf("timeout: %s", c.HTTP.Timeout)
	}
}

func TestProviderAPICallTimeout(t *testing.T) {
	_ = os.Getenv("DUMMY")
}

func TestProviderFromConfigMissingBaseURL(t *testing.T) {
	// "custom" provider has no base URL; without env override it should error.
	t.Setenv("SIN_LLM_BASE_URL", "")
	_, err := ProviderFromConfig("custom", "", "", "", 0)
	if err == nil {
		t.Fatal("expected error for missing base URL")
	}
}

func TestFable5ProviderEntry(t *testing.T) {
	p, ok := Providers["fable"]
	if !ok {
		t.Fatal("missing 'fable' provider entry")
	}
	if p.DefaultModel != ClaudeFable5Model {
		t.Errorf("fable default model: got %s, want %s", p.DefaultModel, ClaudeFable5Model)
	}
	if p.BaseURL != "https://api.anthropic.com/v1" {
		t.Errorf("fable base URL: %s", p.BaseURL)
	}
	if p.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("fable API key env: %s", p.APIKeyEnv)
	}
}

func TestMythos5ProviderEntry(t *testing.T) {
	p, ok := Providers["mythos"]
	if !ok {
		t.Fatal("missing 'mythos' provider entry")
	}
	if p.DefaultModel != ClaudeMythos5Model {
		t.Errorf("mythos default model: got %s, want %s", p.DefaultModel, ClaudeMythos5Model)
	}
	if p.BaseURL != "https://api.anthropic.com/v1" {
		t.Errorf("mythos base URL: %s", p.BaseURL)
	}
	if p.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("mythos API key env: %s", p.APIKeyEnv)
	}
}

func TestModelRegistryFable5(t *testing.T) {
	info, ok := LookupModel(ClaudeFable5Model)
	if !ok {
		t.Fatalf("model %s not in registry", ClaudeFable5Model)
	}
	if info.MaxContext != ClaudeFable5Context {
		t.Errorf("fable max context: got %d, want %d", info.MaxContext, ClaudeFable5Context)
	}
	if info.MaxOutput != ClaudeFable5MaxOut {
		t.Errorf("fable max output: got %d, want %d", info.MaxOutput, ClaudeFable5MaxOut)
	}
	if !info.RequiresThinking {
		t.Error("fable 5 should require extended thinking")
	}
	if info.InputPer1M != 10.0 || info.OutputPer1M != 50.0 {
		t.Errorf("fable pricing: input=%.1f output=%.1f", info.InputPer1M, info.OutputPer1M)
	}
}

func TestModelRegistryMythos5(t *testing.T) {
	info, ok := LookupModel(ClaudeMythos5Model)
	if !ok {
		t.Fatalf("model %s not in registry", ClaudeMythos5Model)
	}
	if info.MaxContext != ClaudeMythos5Context {
		t.Errorf("mythos max context: got %d, want %d", info.MaxContext, ClaudeMythos5Context)
	}
	if info.MaxOutput != ClaudeMythos5MaxOut {
		t.Errorf("mythos max output: got %d, want %d", info.MaxOutput, ClaudeMythos5MaxOut)
	}
	if !info.RequiresThinking {
		t.Error("mythos 5 should require extended thinking")
	}
}

func TestResolveModelFableAliases(t *testing.T) {
	for _, alias := range []string{"fable", "fable5"} {
		if got := ResolveModel(alias); got != ClaudeFable5Model {
			t.Errorf("alias %q: got %q, want %q", alias, got, ClaudeFable5Model)
		}
	}
}

func TestResolveModelMythosAliases(t *testing.T) {
	for _, alias := range []string{"mythos", "mythos5"} {
		if got := ResolveModel(alias); got != ClaudeMythos5Model {
			t.Errorf("alias %q: got %q, want %q", alias, got, ClaudeMythos5Model)
		}
	}
}

func TestProviderFromConfigFable(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	c, err := ProviderFromConfig("fable", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.BaseURL, "api.anthropic.com") {
		t.Errorf("fable base URL: %s", c.BaseURL)
	}
	if c.APIKey != "test-key" {
		t.Errorf("fable api key: %s", c.APIKey)
	}
}

func TestProviderFromConfigMythos(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	c, err := ProviderFromConfig("mythos", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.BaseURL, "api.anthropic.com") {
		t.Errorf("mythos base URL: %s", c.BaseURL)
	}
	if c.APIKey != "test-key" {
		t.Errorf("mythos api key: %s", c.APIKey)
	}
}

func TestLookupModelUnknown(t *testing.T) {
	_, ok := LookupModel("nonexistent-model")
	if ok {
		t.Error("expected false for unknown model lookup")
	}
}
