// SPDX-License-Identifier: MIT
// Purpose: agent registry — loads default agents, merges user agents from
// ~/.config/sin-code/agents/{name}/agent.toml, picks the right agent for a
// task type. Uses NIMAgent when SIN_NIM_API_KEY is set, MockAgent otherwise.
package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

type Registry struct {
	agents map[string]Agent
	config map[string]AgentConfig
}

func NewRegistry(agents []Agent) *Registry {
	r := &Registry{
		agents: map[string]Agent{},
		config: map[string]AgentConfig{},
	}
	for _, a := range agents {
		r.agents[a.Name()] = a
		r.config[a.Name()] = a.Config()
	}
	return r
}

func (r *Registry) Register(a Agent) {
	r.agents[a.Name()] = a
	r.config[a.Name()] = a.Config()
}

func (r *Registry) Get(name string) (Agent, bool) {
	a, ok := r.agents[name]
	return a, ok
}

func (r *Registry) ForType(tt TaskType) (Agent, bool) {
	for _, cfg := range r.config {
		if cfg.Type == tt {
			return r.agents[cfg.Name], true
		}
	}
	return nil, false
}

func (r *Registry) List() []AgentConfig {
	out := make([]AgentConfig, 0, len(r.config))
	for _, c := range r.config {
		out = append(out, c)
	}
	return out
}

func DefaultUserAgentsPath() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cfg, "sin-code", "agents")
}

func LoadUserAgents(baseDir string) ([]AgentConfig, error) {
	if baseDir == "" {
		baseDir = DefaultUserAgentsPath()
	}
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil, nil
	}
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
	}
	var out []AgentConfig
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfgPath := filepath.Join(baseDir, e.Name(), "agent.toml")
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			continue
		}
		var cfg AgentConfig
		if _, err := toml.DecodeFile(cfgPath, &cfg); err != nil {
			return nil, fmt.Errorf("decode %s: %w", cfgPath, err)
		}
		if cfg.Name == "" {
			cfg.Name = e.Name()
		}
		out = append(out, cfg)
	}
	return out, nil
}

func UseNIM() bool {
	return os.Getenv("SIN_NIM_API_KEY") != ""
}

func defaultAgentFactory(cfg AgentConfig) Agent {
	if UseLLM() {
		return NewLLMAgent(cfg)
	}
	return NewMockAgent(cfg)
}

// UseLLM returns true if any LLM provider is configured.
func UseLLM() bool {
	for _, p := range llm.Providers {
		if p.APIKeyEnv != "" {
			if os.Getenv(p.APIKeyEnv) != "" {
				return true
			}
		}
	}
	if os.Getenv("SIN_LLM_API_KEY") != "" {
		return true
	}
	return false
}

func NewRegistryWithDefaults(extraConfigs []AgentConfig) *Registry {
	defaults := DefaultAgents()
	agents := make([]Agent, 0, len(defaults))
	byName := map[string]AgentConfig{}
	for _, c := range defaults {
		byName[c.Name] = c
	}
	for _, c := range extraConfigs {
		if existing, ok := byName[c.Name]; ok {
			merged := mergeConfig(existing, c)
			byName[c.Name] = merged
		} else {
			byName[c.Name] = c
		}
	}
	for _, c := range byName {
		agents = append(agents, defaultAgentFactory(c))
	}
	return NewRegistry(agents)
}

func mergeConfig(base, override AgentConfig) AgentConfig {
	if override.Description != "" {
		base.Description = override.Description
	}
	if override.Model != "" {
		base.Model = override.Model
	}
	if override.MaxTokens != 0 {
		base.MaxTokens = override.MaxTokens
	}
	if override.Temperature != 0 {
		base.Temperature = override.Temperature
	}
	if override.SystemFile != "" {
		base.SystemFile = override.SystemFile
	}
	if override.MaxContext != 0 {
		base.MaxContext = override.MaxContext
	}
	if len(override.ToolsAllow) > 0 {
		base.ToolsAllow = override.ToolsAllow
	}
	if len(override.ToolsDeny) > 0 {
		base.ToolsDeny = override.ToolsDeny
	}
	if override.MemoryNS != "" {
		base.MemoryNS = override.MemoryNS
	}
	if override.RetentionDays != 0 {
		base.RetentionDays = override.RetentionDays
	}
	return base
}
