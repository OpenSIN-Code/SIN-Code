package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type ModelInfo struct {
	ID          string
	Name        string
	Provider    string
	Latency     float64
	CostPer1K   float64
	QualityScore float64
	Description string
}

type ModelSelectorState struct {
	Open      bool
	Query     string
	Sel       int
	Models    []ModelInfo
	Indices   []int
	CurrentID string
}

var DefaultModels = []ModelInfo{
	{
		ID:           "qwen3-coder-plus",
		Name:         "Qwen3 Coder Plus",
		Provider:     "Fireworks AI",
		Latency:      1.2,
		CostPer1K:    0.003,
		QualityScore: 0.92,
		Description:  "Best for coding tasks, high accuracy",
	},
	{
		ID:           "qwen3-235b-a22b",
		Name:         "Qwen3 235B",
		Provider:     "Fireworks AI",
		Latency:      1.8,
		CostPer1K:    0.005,
		QualityScore: 0.89,
		Description:  "Large model, balanced performance",
	},
	{
		ID:           "deepseek-v3",
		Name:         "DeepSeek V3",
		Provider:     "Fireworks AI",
		Latency:      1.5,
		CostPer1K:    0.004,
		QualityScore: 0.87,
		Description:  "Good balance of speed and quality",
	},
	{
		ID:           "llama-3.3-70b",
		Name:         "Llama 3.3 70B",
		Provider:     "Fireworks AI",
		Latency:      0.9,
		CostPer1K:    0.002,
		QualityScore: 0.82,
		Description:  "Fast inference, lower cost",
	},
	{
		ID:           "gpt-4o",
		Name:         "GPT-4o",
		Provider:     "OpenAI",
		Latency:      2.1,
		CostPer1K:    0.010,
		QualityScore: 0.95,
		Description:  "Highest quality, premium pricing",
	},
}

func RenderModelSelector(state ModelSelectorState, styles Styles, width, height int) string {
	if !state.Open {
		return ""
	}

	var b strings.Builder

	b.WriteString(styles.AccentText.Render(" Model Selector"))
	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(strings.Repeat("─", min(width-2, 70))))
	b.WriteString("\n")

	if state.Query != "" {
		b.WriteString(styles.Content.Render(" 🔍 " + state.Query))
	} else {
		b.WriteString(styles.Muted.Render(" 🔍 (type to filter, current: " + state.CurrentID + ")"))
	}
	b.WriteString("\n\n")

	if len(state.Indices) == 0 {
		b.WriteString(styles.Muted.Render("  No models found"))
		b.WriteString("\n")
		return b.String()
	}

	maxVisible := min(8, len(state.Indices))
	start := 0
	if state.Sel >= maxVisible {
		start = state.Sel - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(state.Indices) {
		end = len(state.Indices)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	for displayIdx, modelIdx := range state.Indices[start:end] {
		model := state.Models[modelIdx]
		
		name := model.Name
		if len(name) > 25 {
			name = name[:22] + "..."
		}
		
		latencyStr := fmt.Sprintf("%.1fs", model.Latency)
		costStr := fmt.Sprintf("$%.3f", model.CostPer1K)
		qualityStr := fmt.Sprintf("%.0f%%", model.QualityScore*100)
		
		marker := " "
		if model.ID == state.CurrentID {
			marker = "✓"
		}
		
		line := fmt.Sprintf("  %s %-25s  %8s  %6s  %5s", marker, name, latencyStr, costStr, qualityStr)
		
		if displayIdx+start == state.Sel {
			b.WriteString(styles.SidebarSel.Render(padRight(line, width-4)))
		} else {
			b.WriteString(styles.Content.Render(line))
		}
		b.WriteString("\n")
	}

	if state.Sel >= 0 && state.Sel < len(state.Indices) {
		model := state.Models[state.Indices[state.Sel]]
		b.WriteString("\n")
		b.WriteString(styles.AccentText.Render("▸ "))
		b.WriteString(styles.Bold.Render(model.Name))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render("  " + model.Description))
		b.WriteString("\n")
		b.WriteString(styles.Muted.Render(fmt.Sprintf("  Provider: %s · Latency: %.1fs · Cost: $%.3f/1K · Quality: %.0f%%",
			model.Provider, model.Latency, model.CostPer1K, model.QualityScore*100)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Muted.Render(" ↑/↓ navigate · enter select · esc close"))
	b.WriteString("\n")

	return b.String()
}

func (m *Model) OpenModelSelector() {
	m.ModelSelector.Open = true
	m.ModelSelector.Query = ""
	m.ModelSelector.Sel = 0
	m.ModelSelector.Models = DefaultModels
	m.ModelSelector.Indices = make([]int, len(DefaultModels))
	for i := range DefaultModels {
		m.ModelSelector.Indices[i] = i
	}
	
	for _, entry := range m.Config {
		if entry.Key == "llm.model" {
			m.ModelSelector.CurrentID = entry.Value
			break
		}
	}
	
	m.Mode = ModeModelSelector
}

func (m *Model) CloseModelSelector() {
	m.ModelSelector.Open = false
	m.ModelSelector.Query = ""
	m.ModelSelector.Sel = 0
	m.Mode = ModeNormal
}

func (m *Model) ModelSelectorNavigate(direction int) {
	if len(m.ModelSelector.Indices) == 0 {
		return
	}
	
	m.ModelSelector.Sel += direction
	if m.ModelSelector.Sel < 0 {
		m.ModelSelector.Sel = len(m.ModelSelector.Indices) - 1
	}
	if m.ModelSelector.Sel >= len(m.ModelSelector.Indices) {
		m.ModelSelector.Sel = 0
	}
}

func (m *Model) ModelSelectorSelect() {
	if len(m.ModelSelector.Indices) == 0 {
		return
	}

	if m.ModelSelector.Sel >= 0 && m.ModelSelector.Sel < len(m.ModelSelector.Indices) {
		modelIdx := m.ModelSelector.Indices[m.ModelSelector.Sel]
		model := m.ModelSelector.Models[modelIdx]

		for i, entry := range m.Config {
			if entry.Key == "llm.model" {
				m.Config[i].Value = model.ID
				break
			}
		}

		m.ModelSelector.CurrentID = model.ID

		// Set env var so new runners pick up the model
		os.Setenv("SIN_LLM_MODEL", model.ID)

		// Reset runners so they reinitialize with the new model
		m.ChatRunner = nil
		m.AgentRunner = nil

		// Update footer to show the new model name
		m.Footer.ModelName = model.Name

		m.AppendHistory("Config", "model-change", model.Name, true)
	}

	m.CloseModelSelector()
}

func (m *Model) ModelSelectorFilter(query string) {
	m.ModelSelector.Query = query
	
	if query == "" {
		m.ModelSelector.Indices = make([]int, len(m.ModelSelector.Models))
		for i := range m.ModelSelector.Models {
			m.ModelSelector.Indices[i] = i
		}
		m.ModelSelector.Sel = 0
		return
	}
	
	var filtered []int
	queryLower := strings.ToLower(query)
	for i, model := range m.ModelSelector.Models {
		if strings.Contains(strings.ToLower(model.Name), queryLower) ||
		   strings.Contains(strings.ToLower(model.Provider), queryLower) ||
		   strings.Contains(strings.ToLower(model.Description), queryLower) {
			filtered = append(filtered, i)
		}
	}
	
	m.ModelSelector.Indices = filtered
	m.ModelSelector.Sel = 0
}

func (m *Model) handleModelSelectorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "ctrl+m":
		m.CloseModelSelector()
		return m, nil
	case "enter":
		m.ModelSelectorSelect()
		return m, nil
	case "up":
		m.ModelSelectorNavigate(-1)
		return m, nil
	case "down":
		m.ModelSelectorNavigate(1)
		return m, nil
	case "backspace":
		if len(m.ModelSelector.Query) > 0 {
			m.ModelSelectorFilter(m.ModelSelector.Query[:len(m.ModelSelector.Query)-1])
		}
		return m, nil
	default:
		if len(msg.String()) == 1 {
			m.ModelSelectorFilter(m.ModelSelector.Query + msg.String())
		}
		return m, nil
	}
}
