// SPDX-License-Identifier: MIT
// Purpose: plugin manifest format + validator. Plugins live in
// ~/.local/share/sin-code/plugins/<name>/ with a plugin.toml file describing
// their subcommands, agents, and tools.
package plugins

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const ManifestFile = "plugin.toml"

type Plugin struct {
	Name        string           `toml:"name"`
	Version     string           `toml:"version"`
	Description string           `toml:"description"`
	Author      string           `toml:"author"`
	Homepage    string           `toml:"homepage"`
	License     string           `toml:"license"`
	MinSinCode  string           `toml:"min_sin_code"`
	Capabilities []string        `toml:"capabilities"`
	Subcommands []PluginSubcmd   `toml:"subcommand"`
	Agents      []PluginAgent    `toml:"agents"`
	Tools       []PluginTool     `toml:"tools"`
	Hooks       []PluginHook     `toml:"hooks"`

	Enabled bool   `toml:"-"`
	Path    string `toml:"-"`
}

type PluginSubcmd struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Binary      string   `toml:"binary"`
	Args        []string `toml:"args"`
	Env         []string `toml:"env"`
}

type PluginAgent struct {
	Name        string `toml:"name"`
	Type        string `toml:"type"`
	Model       string `toml:"model"`
	System      string `toml:"system_file"`
	Provider    string `toml:"provider"`
}

type PluginTool struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Binary      string `toml:"binary"`
	Args        []string `toml:"args"`
	Timeout     int    `toml:"timeout"`
}

type PluginHook struct {
	Event   string `toml:"event"`
	Command string `toml:"command"`
	Timeout int    `toml:"timeout"`
}

func (p *Plugin) Validate() error {
	if p.Name == "" {
		return errors.New("plugin.name is required")
	}
	if strings.ContainsAny(p.Name, "/\\. ") {
		return fmt.Errorf("plugin.name %q must not contain / \\ . or space", p.Name)
	}
	if p.Version == "" {
		return errors.New("plugin.version is required")
	}
	for i, s := range p.Subcommands {
		if s.Name == "" {
			return fmt.Errorf("subcommand[%d].name is required", i)
		}
		if s.Binary == "" {
			return fmt.Errorf("subcommand[%d].binary is required", i)
		}
	}
	for i, a := range p.Agents {
		if a.Name == "" {
			return fmt.Errorf("agent[%d].name is required", i)
		}
		if a.Type == "" {
			return fmt.Errorf("agent[%d].type is required", i)
		}
	}
	for i, t := range p.Tools {
		if t.Name == "" {
			return fmt.Errorf("tool[%d].name is required", i)
		}
		if t.Binary == "" {
			return fmt.Errorf("tool[%d].binary is required", i)
		}
	}
	return nil
}

func DefaultPluginDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "sin-code", "plugins")
}

func Load(path string) (*Plugin, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Plugin
	if err := toml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	p.Path = filepath.Dir(path)
	return &p, nil
}

func LoadDir(dir string) ([]*Plugin, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*Plugin
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(dir, e.Name(), ManifestFile)
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}
		p, err := Load(manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load plugin %s: %v\n", e.Name(), err)
			continue
		}
		p.Enabled = p.isEnabledOnDisk()
		out = append(out, p)
	}
	return out, nil
}

func (p *Plugin) isEnabledOnDisk() bool {
	if p.Path == "" {
		return true
	}
	if _, err := os.Stat(filepath.Join(p.Path, ".disabled")); err == nil {
		return false
	}
	return true
}

func (p *Plugin) Disable() error {
	if p.Path == "" {
		return errors.New("plugin path unknown")
	}
	return os.WriteFile(filepath.Join(p.Path, ".disabled"), []byte{}, 0o644)
}

func (p *Plugin) Enable() error {
	if p.Path == "" {
		return errors.New("plugin path unknown")
	}
	return os.Remove(filepath.Join(p.Path, ".disabled"))
}

func (p *Plugin) Dir() string {
	return p.Path
}
