// SPDX-License-Identifier: MIT
// Purpose: shared helpers for the agent edit/set/reset/show/doctor CLI.
package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/orchestrator"
)

func mergeAgentConfig(base, override orchestrator.AgentConfig) orchestrator.AgentConfig {
	if override.Description != "" {
		base.Description = override.Description
	}
	if override.Type != "" {
		base.Type = override.Type
	}
	if override.Provider != "" {
		base.Provider = override.Provider
	}
	if override.BaseURL != "" {
		base.BaseURL = override.BaseURL
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

func agentDir(name string) (string, error) {
	if name == "" || name != sanitizeName(name) {
		return "", fmt.Errorf("invalid agent name: %q", name)
	}
	cfg := os.Getenv("SIN_CODE_CONFIG_DIR")
	if cfg == "" {
		cfg = os.Getenv("XDG_CONFIG_HOME")
	}
	if cfg == "" {
		var err error
		cfg, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(cfg, "sin-code", "agents", name), nil
}

func sanitizeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out = append(out, r)
		}
	}
	return string(out)
}

func loadAllEffectiveAgents() ([]orchestrator.AgentConfig, error) {
	defaults := orchestrator.DefaultAgents()
	extras, err := orchestrator.LoadUserAgents("")
	if err != nil {
		return nil, err
	}
	byName := map[string]orchestrator.AgentConfig{}
	for _, c := range defaults {
		byName[c.Name] = c
	}
	for _, c := range extras {
		if existing, ok := byName[c.Name]; ok {
			byName[c.Name] = mergeAgentConfig(existing, c)
		} else {
			byName[c.Name] = c
		}
	}
	out := make([]orchestrator.AgentConfig, 0, len(byName))
	for _, c := range byName {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func loadEffectiveAgent(name string) (orchestrator.AgentConfig, string, error) {
	dir, err := agentDir(name)
	if err != nil {
		return orchestrator.AgentConfig{}, "", err
	}
	cfgPath := filepath.Join(dir, "agent.toml")
	if _, err := os.Stat(cfgPath); err == nil {
		var userCfg orchestrator.AgentConfig
		if _, derr := toml.DecodeFile(cfgPath, &userCfg); derr != nil {
			return orchestrator.AgentConfig{}, "", fmt.Errorf("decode %s: %w", cfgPath, derr)
		}
		for _, def := range orchestrator.DefaultAgents() {
			if def.Name == name {
				return mergeAgentConfig(def, userCfg), "user (overrides default)", nil
			}
		}
		return userCfg, "user (new agent)", nil
	}
	for _, def := range orchestrator.DefaultAgents() {
		if def.Name == name {
			return def, "default", nil
		}
	}
	return orchestrator.AgentConfig{}, "", fmt.Errorf("agent %q not found in defaults or user config", name)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
