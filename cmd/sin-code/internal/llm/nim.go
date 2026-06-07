// SPDX-License-Identifier: MIT
// Purpose: NVIDIA NIM-specific helpers. NIM exposes an OpenAI-compatible
// chat/completions endpoint at https://integrate.api.nvidia.com/v1 and
// serves Anthropic / Llama / Mistral / etc. models under "vendor/name"
// IDs. Friendly aliases like "haiku" or "sonnet" resolve to the full ID.
package llm

const NIMDefaultBaseURL = "https://integrate.api.nvidia.com/v1"

const NIMDefaultModel = "meta/llama-3.1-70b-instruct"

const NIMClaudeModel = "anthropic/claude-3-5-sonnet-20241022"

const NIMHaikuModel = "anthropic/claude-3-5-haiku-20241022"

var NIMModelAliases = map[string]string{
	"haiku":     NIMHaikuModel,
	"sonnet":    NIMClaudeModel,
	"llama-70b": NIMDefaultModel,
	"llama-8b":  "meta/llama-3.1-8b-instruct",
}

func ResolveModel(name string) string {
	if id, ok := NIMModelAliases[name]; ok {
		return id
	}
	return name
}
