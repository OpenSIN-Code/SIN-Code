// SPDX-License-Identifier: MIT
package mcpclient

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverConfigs_IndividualFiles(t *testing.T) {
	dir := t.TempDir()
	oldHook := userConfigDirHook
	userConfigDirHook = func() (string, error) { return dir, nil }
	defer func() { userConfigDirHook = oldHook }()

	serversDir := filepath.Join(dir, "mcp", "servers")
	if err := os.MkdirAll(serversDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serversDir, "foo.json"), []byte(`{"name":"foo","transport":"stdio","command":"echo","args":["hello"]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgs := DiscoverConfigs("")
	if len(cfgs) != 1 {
		t.Fatalf("got %d configs, want 1", len(cfgs))
	}
	if cfgs[0].Name != "foo" {
		t.Errorf("name = %q, want foo", cfgs[0].Name)
	}
}

func TestDiscoverConfigs_MCPServersMap(t *testing.T) {
	dir := t.TempDir()
	oldConfigHook := userConfigDirHook
	oldHomeHook := userHomeDirHook
	userConfigDirHook = func() (string, error) { return dir, nil }
	userHomeDirHook = func() (string, error) { return dir, nil }
	defer func() {
		userConfigDirHook = oldConfigHook
		userHomeDirHook = oldHomeHook
	}()

	claudeDir := filepath.Join(dir, ".config", "claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"mcpServers":{"bar":{"command":"npx","args":["-y","@modelcontextprotocol/server-filesystem","/tmp"]}}}`)
	if err := os.WriteFile(filepath.Join(claudeDir, "claude_desktop_config.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfgs := DiscoverConfigs("")
	if len(cfgs) != 1 {
		t.Fatalf("got %d configs, want 1", len(cfgs))
	}
	if cfgs[0].Name != "bar" {
		t.Errorf("name = %q, want bar", cfgs[0].Name)
	}
	if cfgs[0].Command != "npx" {
		t.Errorf("command = %q, want npx", cfgs[0].Command)
	}
}

func TestDiscoverConfigs_WorkspaceOverride(t *testing.T) {
	dir := t.TempDir()
	ws := t.TempDir()
	oldHook := userConfigDirHook
	userConfigDirHook = func() (string, error) { return dir, nil }
	defer func() { userConfigDirHook = oldHook }()

	serversDir := filepath.Join(dir, "mcp", "servers")
	if err := os.MkdirAll(serversDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serversDir, "baz.json"), []byte(`{"name":"baz","transport":"stdio","command":"echo"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	wsCfg := filepath.Join(ws, ".sin-code", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(wsCfg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wsCfg, []byte(`{"mcpServers":{"baz":{"command":"overridden","args":[]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgs := DiscoverConfigs(ws)
	if len(cfgs) != 1 {
		t.Fatalf("got %d configs, want 1", len(cfgs))
	}
	if cfgs[0].Command != "overridden" {
		t.Errorf("workspace did not override: command = %q", cfgs[0].Command)
	}
}

func TestWriteServerConfig(t *testing.T) {
	dir := t.TempDir()
	oldHook := userConfigDirHook
	userConfigDirHook = func() (string, error) { return dir, nil }
	defer func() { userConfigDirHook = oldHook }()

	if err := WriteServerConfig(ServerConfig{Name: "qux", Transport: "stdio", Command: "echo"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mcp", "servers", "qux.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestDiscoverConfigs_NoFiles(t *testing.T) {
	dir := t.TempDir()
	oldHook := userConfigDirHook
	userConfigDirHook = func() (string, error) { return dir, nil }
	defer func() { userConfigDirHook = oldHook }()

	cfgs := DiscoverConfigs("")
	if len(cfgs) != 0 {
		t.Fatalf("got %d configs, want 0", len(cfgs))
	}
}
