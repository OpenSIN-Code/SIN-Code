//go:build coverage

// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for the remaining error branches in the
// plugin manifest and registry packages.
package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestPluginValidateMissingFields(t *testing.T) {
	cases := []struct {
		name   string
		plugin Plugin
	}{
		{"missing subcommand name", Plugin{Name: "x", Version: "1.0.0", Subcommands: []PluginSubcmd{{Binary: "b"}}}},
		{"missing agent name", Plugin{Name: "x", Version: "1.0.0", Agents: []PluginAgent{{Type: "code"}}}},
		{"missing tool binary", Plugin{Name: "x", Version: "1.0.0", Tools: []PluginTool{{Name: "t"}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.plugin.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDefaultPluginDirError(t *testing.T) {
	orig := userConfigDir
	userConfigDir = func() (string, error) { return "", os.ErrNotExist }
	defer func() { userConfigDir = orig }()
	if got := DefaultPluginDir(); got != "" {
		t.Fatalf("want empty string, got %q", got)
	}
}

func TestLoadReadFileError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.toml")); err == nil {
		t.Fatal("expected read error")
	}
}

func TestLoadDirNotExist(t *testing.T) {
	loaded, err := LoadDir(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded != nil {
		t.Fatalf("want nil, got %v", loaded)
	}
}

func TestLoadDirReadDirError(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDir(f); err == nil {
		t.Fatal("expected read dir error")
	}
}

func TestLoadDirSkipsFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "not-a-dir.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("want 0 plugins, got %d", len(loaded))
	}
}

func TestLoadDirWarnsOnInvalidManifest(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "bad")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "plugin.toml"), []byte("not valid = ="), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Fatalf("want 0 valid plugins, got %d", len(loaded))
	}
}

func TestPluginDisableEnablePathEmpty(t *testing.T) {
	p := &Plugin{}
	if err := p.Disable(); err == nil {
		t.Fatal("expected disable error for empty path")
	}
	if err := p.Enable(); err == nil {
		t.Fatal("expected enable error for empty path")
	}
}

func TestPluginDir(t *testing.T) {
	p := &Plugin{Path: "/tmp/plug"}
	if got := p.Dir(); got != "/tmp/plug" {
		t.Fatalf("Dir = %q", got)
	}
}

func TestPluginIsEnabledOnDiskPathEmpty(t *testing.T) {
	if !(&Plugin{}).isEnabledOnDisk() {
		t.Fatal("empty path should be enabled")
	}
}

func TestRegistryLoadFromDirEmpty(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "sin-code", "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p1Dir := filepath.Join(pluginDir, "p1")
	if err := os.MkdirAll(p1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p1Dir, "plugin.toml"), []byte("name = \"p1\"\nversion = \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prev, had := os.LookupEnv("SIN_CODE_CONFIG_DIR")
	os.Setenv("SIN_CODE_CONFIG_DIR", dir)
	defer func() {
		if had {
			os.Setenv("SIN_CODE_CONFIG_DIR", prev)
		} else {
			os.Unsetenv("SIN_CODE_CONFIG_DIR")
		}
	}()

	r := NewRegistry()
	if err := r.LoadFromDir(""); err != nil {
		t.Fatal(err)
	}
	if got := r.List(); len(got) != 1 {
		t.Fatalf("want 1 plugin, got %d", len(got))
	}
}

func TestRegistryLoadFromDirNotExist(t *testing.T) {
	r := NewRegistry()
	if err := r.LoadFromDir(filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatal(err)
	}
	if got := r.List(); len(got) != 0 {
		t.Fatalf("want 0 plugins, got %d", len(got))
	}
}

func TestRegistryLoadFromDirError(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry()
	if err := r.LoadFromDir(f); err == nil {
		t.Fatal("expected load error")
	}
}

func TestResolvePluginDirDefault(t *testing.T) {
	prev, had := os.LookupEnv("SIN_CODE_CONFIG_DIR")
	if had {
		os.Unsetenv("SIN_CODE_CONFIG_DIR")
	}
	defer func() {
		if had {
			os.Setenv("SIN_CODE_CONFIG_DIR", prev)
		}
	}()
	got := ResolvePluginDir("")
	if !strings.HasSuffix(got, "plugins") {
		t.Fatalf("ResolvePluginDir default should end with plugins, got %q", got)
	}
}

func TestRegistryAddSubcommandsToExecutes(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "exec-plugin")
	binDir := filepath.Join(pluginDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(pluginDir, "ran.marker")
	script := filepath.Join(binDir, "hello")
	body := "#!/bin/sh\ntouch " + marker + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.toml"), []byte(`name = "exec-plugin"
version = "1.0.0"
[[subcommand]]
name = "hello"
description = "Say hi"
binary = "./bin/hello"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatal(err)
	}

	root := &cobra.Command{Use: "root"}
	r.AddSubcommandsTo(root)
	root.SetArgs([]string{"hello"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("subcommand did not run: %v", err)
	}
}

func TestRegistryMCPToolsEmptyDescription(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "tool-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.toml"), []byte(`name = "tool-plugin"
version = "1.0.0"
[[tools]]
name = "noop"
binary = "./bin/noop"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatal(err)
	}
	tools := r.MCPTools()
	if len(tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(tools))
	}
	if !strings.Contains(tools[0].Description, "tool noop") {
		t.Fatalf("unexpected description: %q", tools[0].Description)
	}
}

func TestRunPluginAbsoluteBinary(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "abs.marker")
	if err := runPlugin(dir, "/bin/sh", []string{"-c", "touch " + marker}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("absolute binary did not run: %v", err)
	}
}
