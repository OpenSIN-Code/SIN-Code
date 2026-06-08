// SPDX-License-Identifier: MIT
// Purpose: tests for the plugin manifest format, validator, and registry.
package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPluginValidate(t *testing.T) {
	cases := []struct {
		name    string
		plugin  Plugin
		wantErr bool
	}{
		{"valid minimal", Plugin{Name: "x", Version: "1.0.0"}, false},
		{"missing name", Plugin{Version: "1.0.0"}, true},
		{"missing version", Plugin{Name: "x"}, true},
		{"name with slash", Plugin{Name: "a/b", Version: "1.0.0"}, true},
		{"name with dot", Plugin{Name: "a.b", Version: "1.0.0"}, true},
		{"subcommand missing binary", Plugin{Name: "x", Version: "1.0.0", Subcommands: []PluginSubcmd{{Name: "sc"}}}, true},
		{"subcommand complete", Plugin{Name: "x", Version: "1.0.0", Subcommands: []PluginSubcmd{{Name: "sc", Binary: "bin/sc"}}}, false},
		{"agent missing type", Plugin{Name: "x", Version: "1.0.0", Agents: []PluginAgent{{Name: "a"}}}, true},
		{"agent complete", Plugin{Name: "x", Version: "1.0.0", Agents: []PluginAgent{{Name: "a", Type: "code"}}}, false},
		{"tool missing name", Plugin{Name: "x", Version: "1.0.0", Tools: []PluginTool{{Binary: "bin"}}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.plugin.Validate()
			if c.wantErr && err == nil {
				t.Error("expected error")
			}
			if !c.wantErr && err != nil {
				t.Errorf("unexpected: %v", err)
			}
		})
	}
}

func TestPluginLoad(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "plugin.toml")
	body := `
name = "my-plugin"
version = "1.2.3"
description = "Test plugin"
author = "Alice"
homepage = "https://example.com"
license = "MIT"
min_sin_code = "2.0.0"
capabilities = ["todo", "memory"]

[[subcommand]]
name = "hello"
description = "Say hello"
binary = "./bin/hello"

[[subcommand]]
name = "bye"
description = "Say bye"
binary = "./bin/bye"
args = ["--fast"]

[[agents]]
name = "custom"
type = "code"
model = "openai/gpt-4o"

[[tools]]
name = "mcp-tool"
description = "An MCP tool"
binary = "./bin/mcp"

[[hooks]]
event = "post_complete"
command = "echo done"
`
	if err := os.WriteFile(manifestPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Load(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "my-plugin" {
		t.Errorf("name: %s", p.Name)
	}
	if p.Version != "1.2.3" {
		t.Errorf("version: %s", p.Version)
	}
	if len(p.Subcommands) != 2 {
		t.Errorf("subcommands: %d", len(p.Subcommands))
	}
	if len(p.Agents) != 1 {
		t.Errorf("agents: %d", len(p.Agents))
	}
	if p.Path == "" {
		t.Error("path should be set")
	}
}

func TestPluginLoadInvalid(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "plugin.toml")
	_ = os.WriteFile(manifestPath, []byte("not a valid toml = ="), 0o644)
	if _, err := Load(manifestPath); err == nil {
		t.Error("expected parse error")
	}
}

func TestPluginLoadValidationError(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "plugin.toml")
	_ = os.WriteFile(manifestPath, []byte("version = \"1.0.0\""), 0o644)
	if _, err := Load(manifestPath); err == nil {
		t.Error("expected validation error for missing name")
	}
}

func TestPluginLoadDir(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{"a", "b", "not-a-plugin"} {
		sub := filepath.Join(dir, name)
		_ = os.MkdirAll(sub, 0o755)
		if name == "not-a-plugin" {
			continue
		}
		manifestPath := filepath.Join(sub, "plugin.toml")
		body := "name = \"plug-" + name + "\"\nversion = \"1.0." + string(rune('0'+i)) + "\"\n"
		_ = os.WriteFile(manifestPath, []byte(body), 0o644)
	}
	loaded, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 plugins, got %d", len(loaded))
	}
}

func TestPluginLoadDirMissing(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	_ = os.MkdirAll(sub, 0o755)
	loaded, err := LoadDir(sub)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != nil {
		t.Errorf("expected nil, got %v", loaded)
	}
}

func TestRegistryLoadAndList(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "p1")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "plugin.toml"),
		[]byte("name = \"p1\"\nversion = \"1.0.0\"\n"), 0o644)

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatal(err)
	}
	if len(r.List()) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(r.List()))
	}
	if _, ok := r.Get("p1"); !ok {
		t.Error("expected p1 to be in registry")
	}
}

func TestRegistryAddSubcommandsTo(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "hello-plugin")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "plugin.toml"),
		[]byte(`name = "hello-plugin"
version = "1.0.0"
[[subcommand]]
name = "greet"
description = "Say hi"
binary = "./bin/greet"
`), 0o644)

	r := NewRegistry()
	_ = r.LoadFromDir(dir)

	rootCmd := &cobra.Command{Use: "root"}
	r.AddSubcommandsTo(rootCmd)

	found := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "greet" {
			found = true
			if !strings.Contains(c.Short, "hello-plugin") {
				t.Errorf("short should mention plugin: %s", c.Short)
			}
		}
	}
	if !found {
		t.Error("subcommand not registered with cobra")
	}
}

func TestRegistryAgentConfigs(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "agents-plugin")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "plugin.toml"),
		[]byte(`name = "agents-plugin"
version = "1.0.0"
[[agents]]
name = "fastcoder"
type = "code"
model = "openai/gpt-4o"
`), 0o644)

	r := NewRegistry()
	_ = r.LoadFromDir(dir)
	cfgs := r.AgentConfigs()
	if len(cfgs) != 1 {
		t.Fatalf("expected 1 agent config, got %d", len(cfgs))
	}
	if cfgs[0].Name != "plugin-agents-plugin-fastcoder" {
		t.Errorf("name: %s", cfgs[0].Name)
	}
	if cfgs[0].Model != "openai/gpt-4o" {
		t.Errorf("model: %s", cfgs[0].Model)
	}
}

func TestPluginEnableDisable(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "test-plugin")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "plugin.toml"),
		[]byte("name = \"test-plugin\"\nversion = \"1.0.0\"\n"), 0o644)

	p, err := Load(filepath.Join(sub, "plugin.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !p.isEnabledOnDisk() {
		t.Error("should start enabled")
	}
	if err := p.Disable(); err != nil {
		t.Fatal(err)
	}
	if p.isEnabledOnDisk() {
		t.Error("should be disabled after Disable()")
	}
	if err := p.Enable(); err != nil {
		t.Fatal(err)
	}
	if !p.isEnabledOnDisk() {
		t.Error("should be enabled after Enable()")
	}
}

func TestDefaultPluginDir(t *testing.T) {
	dir := DefaultPluginDir()
	if !strings.HasSuffix(dir, "plugins") {
		t.Errorf("path should end in 'plugins', got %q", dir)
	}
}

func TestRegistryRegisterDirectly(t *testing.T) {
	r := NewRegistry()
	p := &Plugin{Name: "direct", Version: "1.0.0", Path: "/tmp"}
	r.Register(p)
	if got, ok := r.Get("direct"); !ok || got != p {
		t.Error("register failed")
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("nonexistent"); ok {
		t.Error("expected missing")
	}
}
