// SPDX-License-Identifier: MIT
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

const customModelEntry = "Custom…"

type ModelCustomInput struct {
	Open  bool
	Input textinput.Model
}

func NewModelCustomInput() *ModelCustomInput {
	ti := textinput.New()
	ti.Placeholder = "enter model name..."
	ti.CharLimit = 256
	ti.SetWidth(50)
	ti.Focus()
	return &ModelCustomInput{Input: ti}
}

func (c *ModelCustomInput) Open_() {
	c.Open = true
	c.Input.SetValue("")
	c.Input.Focus()
}

func (c *ModelCustomInput) Close() {
	c.Open = false
	c.Input.Blur()
}

func (c *ModelCustomInput) Value() string {
	return strings.TrimSpace(c.Input.Value())
}

func (m *Model) OpenModelCustomInput() {
	if m.ModelCustomInput == nil {
		m.ModelCustomInput = NewModelCustomInput()
	}
	m.ModelCustomInput.Open_()
	m.Mode = ModeModelCustom
}

func (m *Model) CloseModelCustomInput() {
	if m.ModelCustomInput != nil {
		m.ModelCustomInput.Close()
	}
	m.Mode = ModeNormal
}

func (m *Model) handleModelCustomKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ModelCustomInput == nil || !m.ModelCustomInput.Open {
		m.Mode = ModeNormal
		return m, nil
	}
	key := msg.String()
	switch key {
	case "esc":
		m.CloseModelCustomInput()
		return m, nil
	case "enter":
		val := m.ModelCustomInput.Value()
		if val != "" {
			m.applyModelSwitch(val)
		}
		m.CloseModelCustomInput()
		return m, nil
	}
	var cmd tea.Cmd
	m.ModelCustomInput.Input, cmd = m.ModelCustomInput.Input.Update(msg)
	return m, cmd
}

func (m *Model) RenderModelCustomInput(styles Styles, width, height int) string {
	if m.ModelCustomInput == nil || !m.ModelCustomInput.Open {
		return ""
	}
	popupWidth := 54
	if popupWidth > width-4 {
		popupWidth = width - 4
	}
	if popupWidth < 30 {
		popupWidth = 30
	}
	var b strings.Builder
	b.WriteString(styles.AccentText.Render(" Custom Model"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("-", popupWidth-4)))
	b.WriteString("\n\n")
	b.WriteString(styles.Muted.Render("  Enter model identifier:"))
	b.WriteString("\n  ")
	b.WriteString(m.ModelCustomInput.Input.View())
	b.WriteString("\n\n")
	b.WriteString(styles.Muted.Render(" enter confirm | esc cancel"))
	b.WriteString("\n")
	return styles.Popup.Render(b.String())
}

func (m *Model) applyModelSwitch(model string) {
	m.Footer.ModelName = model
	m.AgentConfig.Model = model
	os.Setenv("SIN_LLM_MODEL", model)
	m.ChatRunner = nil
	m.AgentRunner = nil
	m.appendChat(ChatMessage{Kind: chatSystem, Text: fmt.Sprintf("[switched to %s]", model)})
	m.AppendHistory(ViewChat.String(), "model-switch", model, true)
}

func (m *Model) handleModelSwitcherSelect() {
	if m.ModelSwitcher == nil {
		return
	}
	selected := m.ModelSwitcher.Confirm()
	if selected == "" {
		return
	}
	if selected == customModelEntry {
		m.Mode = ModeNormal
		m.OpenModelCustomInput()
		return
	}
	m.applyModelSwitch(selected)
	m.Mode = ModeNormal
}

func buildModelList(m *Model) []string {
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

	for _, entry := range m.Config {
		if entry.Key == "llm.model" && entry.Value != "" {
			add(entry.Value)
		}
	}

	if envModel := os.Getenv("SIN_LLM_MODEL"); envModel != "" {
		add(envModel)
	}

	if m.AgentConfig.Model != "" {
		add(m.AgentConfig.Model)
	}

	if m.Footer.ModelName != "" {
		add(m.Footer.ModelName)
	}

	for _, p := range profileModelPaths() {
		if model := readProfileModel(p); model != "" {
			add(model)
		}
	}

	add("accounts/fireworks/models/qwen3-coder-480b-a35b-instruct")
	add("accounts/fireworks/models/glm-5p2")
	add("nvidia/nemotron-3-ultra-550b-a55b")
	add("nvidia/nemotron-3-super-120b-a12b")
	add("nvidia/nemotron-3-nano-30b-a3b")
	add("meta/llama-3.3-70b-instruct")
	add("moonshotai/kimi-k2.6")
	add("mistralai/mistral-medium-3.5-128b")
	add("openai/gpt-oss-120b")
	add("deepseek-ai/DeepSeek-V3")
	add("meta/llama-3.3-70b-instruct")
	add("qwen3-coder")

	for _, mi := range DefaultModels {
		add(mi.ID)
	}

	out = append(out, customModelEntry)
	return out
}

func profileModelPaths() []string {
	var paths []string
	candidates := []string{
		"profiles/fireworks.toml",
		"profiles/qwen-relay.toml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			paths = append(paths, c)
		}
		home, err := os.UserHomeDir()
		if err == nil {
			hp := filepath.Join(home, ".config", "sin-code", filepath.Base(c))
			if _, err := os.Stat(hp); err == nil {
				paths = append(paths, hp)
			}
		}
	}
	return paths
}

func readProfileModel(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "model") {
			idx := strings.Index(line, "=")
			if idx < 0 {
				continue
			}
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, "\"'")
			return val
		}
	}
	return ""
}
