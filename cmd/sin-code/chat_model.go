// SPDX-License-Identifier: MIT
// Purpose: available model list for the /model slash command. Reads from
// config, env, agent profiles, and known provider defaults.
package main

import (
	"os"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

var defaultChatModels = []string{
	"accounts/fireworks/models/qwen3-coder-480b-a35b-instruct",
	"accounts/fireworks/models/glm-5p2",
	"nvidia/nemotron-3-ultra-550b-a55b",
	"nvidia/nemotron-3-super-120b-a12b",
	"nvidia/nemotron-3-nano-30b-a3b",
	"meta/llama-3.3-70b-instruct",
	"moonshotai/kimi-k2.6",
	"mistralai/mistral-medium-3.5-128b",
	"openai/gpt-oss-120b",
	"deepseek-ai/DeepSeek-V3",
	"qwen3-coder",
	"gpt-4o",
}

func availableChatModels(sinCfg internal.SinCodeConfig, agentCfg orchestrator.AgentConfig) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			return
		}
		seen[model] = true
		out = append(out, model)
	}
	if sinCfg.LLMModel != "" {
		add(sinCfg.LLMModel)
	}
	if envModel := os.Getenv("SIN_LLM_MODEL"); envModel != "" {
		add(envModel)
	}
	if agentCfg.Model != "" {
		add(agentCfg.Model)
	}
	for _, p := range llm.Providers {
		if p.DefaultModel != "" {
			add(p.DefaultModel)
		}
	}
	for _, m := range defaultChatModels {
		add(m)
	}
	return out
}
