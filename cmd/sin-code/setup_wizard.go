// SPDX-License-Identifier: MIT
// Purpose: interactive onboarding wizard for `sin-code chat --setup` and
// `sin-code config init --setup`. Guides first-time users through provider
// selection, API key entry, and model choice.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
)

type providerPreset struct {
	Name        string
	BaseURL     string
	APIKeyEnv   string
	Models      []string
	DefaultModel string
}

var providerPresets = []providerPreset{
	{
		Name:        "Fireworks AI",
		BaseURL:     "https://api.fireworks.ai/inference/v1",
		APIKeyEnv:   "FIREWORKS_API_KEY",
		Models:      []string{"accounts/fireworks/models/qwen3-coder-480b", "accounts/fireworks/models/llama-v3p1-405b-instruct", "accounts/fireworks/models/deepseek-v3"},
		DefaultModel: "accounts/fireworks/models/qwen3-coder-480b",
	},
	{
		Name:        "NVIDIA NIM",
		BaseURL:     "https://integrate.api.nvidia.com/v1",
		APIKeyEnv:   "NVIDIA_API_KEY",
		Models: []string{
			"nvidia/nemotron-3-ultra-550b-a55b",
			"nvidia/nemotron-3-super-120b-a12b",
			"nvidia/nemotron-3-nano-30b-a3b",
			"meta/llama-3.3-70b-instruct",
			"moonshotai/kimi-k2.6",
			"mistralai/mistral-medium-3.5-128b",
			"openai/gpt-oss-120b",
		},
		DefaultModel: "nvidia/nemotron-3-nano-30b-a3b",
	},
	{
		Name:        "OpenAI-compatible (custom base URL)",
		BaseURL:     "",
		APIKeyEnv:   "OPENAI_API_KEY",
		Models:      nil,
		DefaultModel: "",
	},
}

var setupStdin io.Reader = os.Stdin
var setupStdout io.Writer = os.Stdout
var setupStderr io.Writer = os.Stderr

func setupIsTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func runSetupWizard() error {
	if !setupIsTerminal(os.Stdin) {
		fmt.Fprintln(setupStderr, "No LLM configured. Run 'sin-code config init' or set one of:")
		fmt.Fprintln(setupStderr, "  export NVIDIA_API_KEY=...")
		fmt.Fprintln(setupStderr, "  export SIN_LLM_API_KEY=...")
		fmt.Fprintln(setupStderr, "  export OPENAI_API_KEY=...")
		return fmt.Errorf("no TTY available for interactive setup")
	}

	reader := bufio.NewReader(setupStdin)

	fmt.Fprintln(setupStdout, "")
	fmt.Fprintln(setupStdout, "Welcome to SIN-Code! Let's configure your LLM backend.")
	fmt.Fprintln(setupStdout, "")

	fmt.Fprintln(setupStdout, "1. Choose your LLM provider:")
	for i, p := range providerPresets {
		label := p.Name
		if i == 0 {
			label += " (recommended — uses qwen3-coder-480b)"
		}
		fmt.Fprintf(setupStdout, "   [%d] %s\n", i+1, label)
	}
	fmt.Fprintln(setupStdout, "   [4] Skip — I'll configure manually")
	fmt.Fprintln(setupStdout, "")

	choice, err := readPromptLine(reader, "> ")
	if err != nil {
		return fmt.Errorf("setup: read provider choice: %w", err)
	}

	var provider *providerPreset
	switch strings.TrimSpace(choice) {
	case "1", "2", "3":
		provider = &providerPresets[parseChoice(choice)-1]
	case "4", "":
		fmt.Fprintln(setupStdout, "")
		fmt.Fprintln(setupStdout, "Skipped. Edit ~/.config/sin/sin-code.toml manually.")
		fmt.Fprintln(setupStdout, "Run 'sin-code config path' to find the config file.")
		return nil
	default:
		fmt.Fprintf(setupStdout, "Invalid choice %q, skipping setup.\n", choice)
		return fmt.Errorf("setup: invalid provider choice %q", choice)
	}

	fmt.Fprintln(setupStdout, "")

	envVal := os.Getenv(provider.APIKeyEnv)
	promptSuffix := fmt.Sprintf(" (or set %s env var)", provider.APIKeyEnv)
	if envVal != "" {
		promptSuffix = fmt.Sprintf(" (%s env var is set — press Enter to use it)", provider.APIKeyEnv)
	}

	fmt.Fprintf(setupStdout, "2. Enter your %s API key%s:\n", provider.Name, promptSuffix)
	apiKey, err := readPromptLine(reader, "> ")
	if err != nil {
		return fmt.Errorf("setup: read API key: %w", err)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		apiKey = envVal
	}
	if apiKey == "" {
		fmt.Fprintln(setupStdout, "No API key provided. You can set it later via 'sin-code config set llm.api_key <key>'.")
	}

	fmt.Fprintln(setupStdout, "")

	var model string
	if len(provider.Models) > 0 {
		fmt.Fprintln(setupStdout, "3. Choose a model:")
		for i, m := range provider.Models {
			label := m
			if m == provider.DefaultModel {
				label += " (recommended)"
			}
			fmt.Fprintf(setupStdout, "   [%d] %s\n", i+1, label)
		}
		fmt.Fprintln(setupStdout, "   [3] Custom")
		fmt.Fprintln(setupStdout, "")

		modelChoice, err := readPromptLine(reader, "> ")
		if err != nil {
			return fmt.Errorf("setup: read model choice: %w", err)
		}
		switch strings.TrimSpace(modelChoice) {
		case "1", "2":
			idx := parseChoice(modelChoice) - 1
			if idx >= 0 && idx < len(provider.Models) {
				model = provider.Models[idx]
			}
		case "3", "":
			model, err = readPromptLine(reader, "Enter model name: ")
			if err != nil {
				return fmt.Errorf("setup: read custom model: %w", err)
			}
			model = strings.TrimSpace(model)
		default:
			model = provider.DefaultModel
		}
		if model == "" {
			model = provider.DefaultModel
		}
	} else {
		fmt.Fprintln(setupStdout, "3. Enter your base URL:")
		baseURL, err := readPromptLine(reader, "> ")
		if err != nil {
			return fmt.Errorf("setup: read base URL: %w", err)
		}
		provider.BaseURL = strings.TrimSpace(baseURL)

		fmt.Fprintln(setupStdout)
		fmt.Fprintln(setupStdout, "4. Enter model name:")
		model, err = readPromptLine(reader, "> ")
		if err != nil {
			return fmt.Errorf("setup: read model name: %w", err)
		}
		model = strings.TrimSpace(model)
	}

	cfg := internal.DefaultConfig()
	cfg.LLMBaseURL = provider.BaseURL
	cfg.LLMAPIKey = apiKey
	cfg.LLMModel = model

	if err := internal.SaveConfig(cfg); err != nil {
		return fmt.Errorf("setup: save config: %w", err)
	}

	fmt.Fprintln(setupStdout)
	fmt.Fprintln(setupStdout, "Configuration saved to ~/.config/sin/sin-code.toml")
	fmt.Fprintf(setupStdout, "Model: %s\n", model)
	fmt.Fprintf(setupStdout, "Provider: %s\n", provider.Name)
	fmt.Fprintln(setupStdout)
	fmt.Fprintln(setupStdout, "You're ready! Run 'sin-code chat' to start.")
	return nil
}

func readPromptLine(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(setupStdout, prompt)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func parseChoice(s string) int {
	s = strings.TrimSpace(s)
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func init() {
	internal.SetupWizardHook = runSetupWizard
}
