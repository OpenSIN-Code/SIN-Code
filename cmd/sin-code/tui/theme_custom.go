// SPDX-License-Identifier: MIT
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CustomTheme struct {
	Name       string `json:"name"`
	Accent     string `json:"accent"`
	AccentDim  string `json:"accent_dim"`
	Text       string `json:"text"`
	TextDim    string `json:"text_dim"`
	Background string `json:"background"`
	Border     string `json:"border"`
	Success    string `json:"success"`
	Warn       string `json:"warn"`
	Error      string `json:"error"`
}

func CustomThemePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sin-code", "theme.json"), nil
}

func LoadCustomTheme(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read theme file: %w", err)
	}
	var ct CustomTheme
	if err := json.Unmarshal(data, &ct); err != nil {
		return nil, fmt.Errorf("parse theme JSON: %w", err)
	}
	theme := ct.ToTheme()
	if err := theme.Validate(); err != nil {
		return nil, fmt.Errorf("invalid theme: %w", err)
	}
	return &theme, nil
}

func SaveCustomTheme(theme Theme, path string) error {
	ct := CustomTheme{
		Name:       theme.Name,
		Accent:     theme.Accent,
		AccentDim:  theme.AccentDim,
		Text:       theme.Text,
		TextDim:    theme.TextDim,
		Background: theme.Background,
		Border:     theme.Border,
		Success:    theme.Success,
		Warn:       theme.Warn,
		Error:      theme.Error,
	}
	data, err := json.MarshalIndent(ct, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal theme: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create theme dir: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func (ct CustomTheme) ToTheme() Theme {
	return Theme{
		Name:       ct.Name,
		Accent:     ct.Accent,
		AccentDim:  ct.AccentDim,
		Text:       ct.Text,
		TextDim:    ct.TextDim,
		Background: ct.Background,
		Border:     ct.Border,
		Success:    ct.Success,
		Warn:       ct.Warn,
		Error:      ct.Error,
	}
}

func (t *Theme) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("theme name is required")
	}
	colorFields := []struct {
		name  string
		value string
	}{
		{"accent", t.Accent},
		{"accent_dim", t.AccentDim},
		{"text", t.Text},
		{"text_dim", t.TextDim},
		{"background", t.Background},
		{"border", t.Border},
		{"success", t.Success},
		{"warn", t.Warn},
		{"error", t.Error},
	}
	for _, f := range colorFields {
		if f.value == "" {
			return fmt.Errorf("color field %s is empty", f.name)
		}
		if !isValidHexColor(f.value) {
			return fmt.Errorf("color field %s has invalid hex color: %s", f.name, f.value)
		}
	}
	return nil
}

func isValidHexColor(s string) bool {
	if !strings.HasPrefix(s, "#") {
		return false
	}
	hex := s[1:]
	if len(hex) != 3 && len(hex) != 6 && len(hex) != 8 {
		return false
	}
	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func (m *Model) LoadCustomThemeFromPath(path string) error {
	theme, err := LoadCustomTheme(path)
	if err != nil {
		return err
	}
	m.Styles = NewStyles(*theme)
	m.AppendHistory(m.ViewKind.String(), "theme-custom", theme.Name, true)
	return nil
}

func (m *Model) ExportThemeToPath(path string) error {
	theme := Themes[m.ThemeIdx]
	if err := SaveCustomTheme(theme, path); err != nil {
		return err
	}
	m.AppendHistory(m.ViewKind.String(), "theme-export", path, true)
	return nil
}
