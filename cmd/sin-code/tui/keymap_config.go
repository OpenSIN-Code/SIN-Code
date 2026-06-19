package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
)

type KeymapContext string

const (
	CtxGlobal   KeymapContext = "global"
	CtxTools    KeymapContext = "tools"
	CtxChat     KeymapContext = "chat"
	CtxSessions KeymapContext = "sessions"
	CtxConfig   KeymapContext = "config"
	CtxDAG      KeymapContext = "dag"
	CtxPalette  KeymapContext = "palette"
)

var allContexts = []KeymapContext{
	CtxGlobal, CtxTools, CtxChat, CtxSessions, CtxConfig, CtxDAG, CtxPalette,
}

type ContextBindings map[string][]string

type KeymapConfig struct {
	Global   ContextBindings `json:"global"`
	Tools    ContextBindings `json:"tools"`
	Chat     ContextBindings `json:"chat"`
	Sessions ContextBindings `json:"sessions"`
	Config   ContextBindings `json:"config"`
	DAG      ContextBindings `json:"dag"`
	Palette  ContextBindings `json:"palette"`
}

func DefaultKeymapConfig() KeymapConfig {
	km := DefaultKeymap()
	return KeymapConfig{
		Global: ContextBindings{
			"quit":           km.Quit.Keys(),
			"help":           km.Help.Keys(),
			"palette":        km.Palette.Keys(),
			"toggle_sidebar": km.ToggleSidebar.Keys(),
			"cycle_theme":    km.CycleTheme.Keys(),
			"cycle_agent":    km.CycleAgent.Keys(),
			"interrupt":      km.Interrupt.Keys(),
			"next_view":      km.NextView.Keys(),
			"prev_view":      km.PrevView.Keys(),
			"view_tools":     km.ViewTools.Keys(),
			"view_sessions":  km.ViewSessions.Keys(),
			"view_efm":       km.ViewEFM.Keys(),
			"view_config":    km.ViewConfig.Keys(),
			"view_history":   km.ViewHistory.Keys(),
			"view_todos":     km.ViewTodos.Keys(),
			"view_chat":      km.ViewChat.Keys(),
			"view_dag":       km.ViewDAG.Keys(),
			"view_context":   km.ViewContext.Keys(),
			"view_dashboard": km.ViewDashboard.Keys(),
			"view_kanban":    km.ViewKanban.Keys(),
			"model_select":   km.ModelSelect.Keys(),
			"subagents":      km.Subagents.Keys(),
		},
		Tools: ContextBindings{
			"run_tool":  km.RunTool.Keys(),
			"show_help": km.ShowHelp.Keys(),
			"tool_up":   km.ToolUp.Keys(),
			"tool_down": km.ToolDown.Keys(),
		},
		Chat: ContextBindings{
			"submit":         km.Submit.Keys(),
			"cancel":         km.Cancel.Keys(),
			"search":         km.Search.Keys(),
			"copy_message":   km.CopyMessage.Keys(),
			"scroll_up":      km.ScrollUp.Keys(),
			"scroll_down":    km.ScrollDown.Keys(),
			"compact_toggle": km.CompactToggle.Keys(),
		},
		Sessions: ContextBindings{
			"new_session":    km.NewSession.Keys(),
			"close_session":  km.CloseSession.Keys(),
			"session_switch": km.SessionSwitch.Keys(),
		},
		Config:  ContextBindings{},
		DAG:     ContextBindings{},
		Palette: ContextBindings{},
	}
}

func (c KeymapConfig) Context(ctx KeymapContext) ContextBindings {
	switch ctx {
	case CtxGlobal:
		return c.Global
	case CtxTools:
		return c.Tools
	case CtxChat:
		return c.Chat
	case CtxSessions:
		return c.Sessions
	case CtxConfig:
		return c.Config
	case CtxDAG:
		return c.DAG
	case CtxPalette:
		return c.Palette
	}
	return nil
}

func (c KeymapConfig) ToKeymap() Keymap {
	km := DefaultKeymap()
	applyCtx := func(b *key.Binding, ctx ContextBindings, action string) {
		if keys, ok := ctx[action]; ok && len(keys) > 0 {
			help := b.Help()
			*b = key.NewBinding(key.WithKeys(keys...), key.WithHelp(help.Key, help.Desc))
		}
	}
	g := c.Global
	applyCtx(&km.Quit, g, "quit")
	applyCtx(&km.Help, g, "help")
	applyCtx(&km.Palette, g, "palette")
	applyCtx(&km.ToggleSidebar, g, "toggle_sidebar")
	applyCtx(&km.CycleTheme, g, "cycle_theme")
	applyCtx(&km.CycleAgent, g, "cycle_agent")
	applyCtx(&km.Interrupt, g, "interrupt")
	applyCtx(&km.NextView, g, "next_view")
	applyCtx(&km.PrevView, g, "prev_view")
	applyCtx(&km.ViewTools, g, "view_tools")
	applyCtx(&km.ViewSessions, g, "view_sessions")
	applyCtx(&km.ViewEFM, g, "view_efm")
	applyCtx(&km.ViewConfig, g, "view_config")
	applyCtx(&km.ViewHistory, g, "view_history")
	applyCtx(&km.ViewTodos, g, "view_todos")
	applyCtx(&km.ViewChat, g, "view_chat")
	applyCtx(&km.ViewDAG, g, "view_dag")
	applyCtx(&km.ViewContext, g, "view_context")
	applyCtx(&km.ViewDashboard, g, "view_dashboard")
	applyCtx(&km.ViewKanban, g, "view_kanban")
	applyCtx(&km.ModelSelect, g, "model_select")
	applyCtx(&km.Subagents, g, "subagents")

	applyCtx(&km.RunTool, c.Tools, "run_tool")
	applyCtx(&km.ShowHelp, c.Tools, "show_help")
	applyCtx(&km.ToolUp, c.Tools, "tool_up")
	applyCtx(&km.ToolDown, c.Tools, "tool_down")

	applyCtx(&km.Submit, c.Chat, "submit")
	applyCtx(&km.Cancel, c.Chat, "cancel")
	applyCtx(&km.Search, c.Chat, "search")
	applyCtx(&km.CopyMessage, c.Chat, "copy_message")
	applyCtx(&km.ScrollUp, c.Chat, "scroll_up")
	applyCtx(&km.ScrollDown, c.Chat, "scroll_down")
	applyCtx(&km.CompactToggle, c.Chat, "compact_toggle")

	applyCtx(&km.NewSession, c.Sessions, "new_session")
	applyCtx(&km.CloseSession, c.Sessions, "close_session")
	applyCtx(&km.SessionSwitch, c.Sessions, "session_switch")

	return km
}

func KeymapConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sin-code", "keymap.json"), nil
}

func LoadKeymapConfig(path string) (KeymapConfig, error) {
	var cfg KeymapConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse keymap config: %w", err)
	}
	if cfg.Global == nil {
		cfg.Global = ContextBindings{}
	}
	if cfg.Tools == nil {
		cfg.Tools = ContextBindings{}
	}
	if cfg.Chat == nil {
		cfg.Chat = ContextBindings{}
	}
	if cfg.Sessions == nil {
		cfg.Sessions = ContextBindings{}
	}
	if cfg.Config == nil {
		cfg.Config = ContextBindings{}
	}
	if cfg.DAG == nil {
		cfg.DAG = ContextBindings{}
	}
	if cfg.Palette == nil {
		cfg.Palette = ContextBindings{}
	}
	return cfg, nil
}

func (c KeymapConfig) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create keymap config dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal keymap config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

type KeyConflict struct {
	Context KeymapContext
	Key     string
	Actions []string
}

func (c KeymapConfig) DetectConflicts() []KeyConflict {
	var conflicts []KeyConflict
	for _, ctx := range allContexts {
		bindings := c.Context(ctx)
		if bindings == nil {
			continue
		}
		keyToActions := map[string][]string{}
		for action, keys := range bindings {
			for _, k := range keys {
				keyToActions[k] = append(keyToActions[k], action)
			}
		}
		for k, actions := range keyToActions {
			if len(actions) > 1 {
				sort.Strings(actions)
				conflicts = append(conflicts, KeyConflict{
					Context: ctx,
					Key:     k,
					Actions: actions,
				})
			}
		}
	}
	return conflicts
}

func (c KeymapConfig) Merge(other KeymapConfig) KeymapConfig {
	merged := c
	mergeCtx := func(dst, src ContextBindings) ContextBindings {
		if dst == nil {
			dst = ContextBindings{}
		}
		for action, keys := range src {
			if len(keys) > 0 {
				dst[action] = keys
			}
		}
		return dst
	}
	merged.Global = mergeCtx(merged.Global, other.Global)
	merged.Tools = mergeCtx(merged.Tools, other.Tools)
	merged.Chat = mergeCtx(merged.Chat, other.Chat)
	merged.Sessions = mergeCtx(merged.Sessions, other.Sessions)
	merged.Config = mergeCtx(merged.Config, other.Config)
	merged.DAG = mergeCtx(merged.DAG, other.DAG)
	merged.Palette = mergeCtx(merged.Palette, other.Palette)
	return merged
}

func (c KeymapConfig) AllContexts() []KeymapContext {
	return allContexts
}

func (c KeymapConfig) Summary() string {
	var b strings.Builder
	for _, ctx := range allContexts {
		bindings := c.Context(ctx)
		b.WriteString(fmt.Sprintf("%s: %d bindings\n", ctx, len(bindings)))
	}
	return b.String()
}

var VimPreset = KeymapConfig{
	Chat: ContextBindings{
		"scroll_up":   []string{"k"},
		"scroll_down": []string{"j"},
	},
	Tools: ContextBindings{
		"tool_up":   []string{"k"},
		"tool_down": []string{"j"},
	},
}
