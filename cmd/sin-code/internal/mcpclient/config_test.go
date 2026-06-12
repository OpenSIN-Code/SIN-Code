// SPDX-License-Identifier: MIT
// Purpose: tests for mcp.json discovery: merge order, disabled flag,
// both file shapes, env expansion, broken-file resilience.
package mcpclient

import (
	"os"
	"path/filepath"
	"testing"
)

func writeWorkspaceConfig(t *testing.T, ws, content string) {
	t.Helper()
	dir := filepath.Join(ws, ".sin-code")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findConfig(cfgs []ServerConfig, name string) *ServerConfig {
	for i := range cfgs {
		if cfgs[i].Name == name {
			return &cfgs[i]
		}
	}
	return nil
}

func TestDefaultsPresentWithoutConfigFiles(t *testing.T) {
	cfgs := LoadConfigs(t.TempDir())
	if findConfig(cfgs, "websearch") == nil || findConfig(cfgs, "browser") == nil {
		t.Fatalf("expected default ecosystem servers, got %d entries", len(cfgs))
	}
}

func TestWorkspaceOverridesDefaultAndMapShape(t *testing.T) {
	ws := t.TempDir()
	writeWorkspaceConfig(t, ws, `{"mcpServers":{
		"websearch": {"transport":"http","url":"http://localhost:9999/mcp"},
		"custom":    {"transport":"stdio","command":"my-server"}
	}}`)
	cfgs := LoadConfigs(ws)
	wsrv := findConfig(cfgs, "websearch")
	if wsrv == nil || wsrv.Transport != "http" || wsrv.URL != "http://localhost:9999/mcp" {
		t.Fatalf("workspace override not applied: %+v", wsrv)
	}
	if findConfig(cfgs, "custom") == nil {
		t.Fatal("custom server missing")
	}
}

func TestDisabledRemovesServer(t *testing.T) {
	ws := t.TempDir()
	writeWorkspaceConfig(t, ws, `{"mcpServers":{"browser":{"disabled":true}}}`)
	if findConfig(LoadConfigs(ws), "browser") != nil {
		t.Fatal("disabled server must be removed")
	}
}

func TestArrayShapeAndEnvExpansion(t *testing.T) {
	t.Setenv("SIN_TEST_PORT", "7777")
	ws := t.TempDir()
	writeWorkspaceConfig(t, ws,
		`[{"name":"arrsrv","transport":"http","url":"http://localhost:${SIN_TEST_PORT}/mcp"}]`)
	srv := findConfig(LoadConfigs(ws), "arrsrv")
	if srv == nil || srv.URL != "http://localhost:7777/mcp" {
		t.Fatalf("env expansion failed: %+v", srv)
	}
}

func TestBrokenConfigIsSkippedNotFatal(t *testing.T) {
	ws := t.TempDir()
	writeWorkspaceConfig(t, ws, `{not json`)
	if findConfig(LoadConfigs(ws), "websearch") == nil {
		t.Fatal("broken workspace config must not wipe defaults")
	}
}
