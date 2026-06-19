// SPDX-License-Identifier: MIT
package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type SessionState struct {
	ViewKind         int           `json:"view_kind"`
	ChatHistory      []ChatMessage `json:"chat_history"`
	SessionID        string        `json:"session_id"`
	ModelName        string        `json:"model_name"`
	ThemeIndex       int           `json:"theme_index"`
	SidebarCollapsed bool          `json:"sidebar_collapsed"`
	SplitPaneActive  bool          `json:"split_pane_active"`
	SavedAt          time.Time     `json:"saved_at"`
}

type CrashRecovery struct {
	path string
}

func crashRecoveryPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".local", "share", "sin-code")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "tui-state.json")
}

func NewCrashRecovery() *CrashRecovery {
	return &CrashRecovery{path: crashRecoveryPath()}
}

func NewCrashRecoveryWithPath(path string) *CrashRecovery {
	return &CrashRecovery{path: path}
}

func (r *CrashRecovery) Path() string { return r.path }

func (r *CrashRecovery) Save(state SessionState) error {
	state.SavedAt = time.Now()
	if len(state.ChatHistory) > 50 {
		state.ChatHistory = state.ChatHistory[len(state.ChatHistory)-50:]
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}

func (r *CrashRecovery) Load() (*SessionState, error) {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return nil, err
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *CrashRecovery) Clear() error {
	if !r.Exists() {
		return nil
	}
	return os.Remove(r.path)
}

func (r *CrashRecovery) Exists() bool {
	_, err := os.Stat(r.path)
	return err == nil
}

func (m *Model) SaveCrashState() {
	if m.CrashRecovery == nil {
		m.CrashRecovery = NewCrashRecovery()
	}
	state := SessionState{
		ViewKind: int(m.ViewKind), ChatHistory: m.ChatHistory,
		ModelName: m.Footer.ModelName, ThemeIndex: m.ThemeIdx,
		SidebarCollapsed: m.Sidebar.Collapsed,
		SplitPaneActive:  m.SplitPane != nil && m.SplitPane.Active(),
	}
	_ = m.CrashRecovery.Save(state)
}

func (m *Model) RestoreCrashState(state *SessionState) {
	if state == nil {
		return
	}
	if state.ViewKind >= 0 && state.ViewKind < viewCount {
		m.SwitchView(ViewKind(state.ViewKind))
	}
	if len(state.ChatHistory) > 0 {
		m.ChatHistory = state.ChatHistory
	}
	if state.ModelName != "" {
		m.Footer.ModelName = state.ModelName
		m.AgentConfig.Model = state.ModelName
	}
	if state.ThemeIndex >= 0 && state.ThemeIndex < len(Themes) {
		m.ThemeIdx = state.ThemeIndex
		m.ApplyTheme()
	}
	if state.SidebarCollapsed != m.Sidebar.Collapsed {
		m.Sidebar.Toggle()
	}
	if m.SplitPane != nil {
		m.SplitPane.SetActive(state.SplitPaneActive)
	}
}

func RecoverAndSave(model *Model) {
	if r := recover(); r != nil {
		if model != nil {
			model.SaveCrashState()
		}
		panic(r)
	}
}
