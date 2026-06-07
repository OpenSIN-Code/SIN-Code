// SPDX-License-Identifier: MIT
// Purpose: NVIDIA NIM-specific helpers. NIM exposes an OpenAI-compatible
// chat/completions endpoint at https://integrate.api.nvidia.com/v1 and
// serves many vendor models. Friendly aliases like "haiku" or "qwen"
// resolve to a working NIM model ID.
package llm

const NIMDefaultBaseURL = "https://integrate.api.nvidia.com/v1"

const NIMDefaultModel = "meta/llama-3.3-70b-instruct"

const NIMHaikuModel = "nvidia/llama-3.1-nemotron-nano-8b-v1"

const NIMKimiModel = "moonshotai/kimi-k2.6"

const NIMQwenModel = "qwen/qwen3-coder-480b-a35b-instruct"

const NIMNemotronModel = "nvidia/nemotron-3-nano-30b-a3b"

const NIMGptOssModel = "openai/gpt-oss-120b"

var NIMModelAliases = map[string]string{
	"haiku":         NIMHaikuModel,
	"kimi":          NIMKimiModel,
	"qwen":          NIMQwenModel,
	"nemotron":      NIMNemotronModel,
	"gpt-oss":       NIMGptOssModel,
	"llama-70b":     NIMDefaultModel,
	"llama-3.3-70b": NIMDefaultModel,
	"llama-8b":      "meta/llama-3.1-8b-instruct",
	"default":       NIMDefaultModel,
}

func ResolveModel(name string) string {
	if id, ok := NIMModelAliases[name]; ok {
		return id
	}
	return name
}
