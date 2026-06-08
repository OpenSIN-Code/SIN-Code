// SPDX-License-Identifier: MIT
// Purpose: tests for the registry's wiring helpers (AgentConfigs, MCPTools,
// HooksFor) and for the firePluginHooks integration. These tests verify the
// contract that the orchestrator / MCP server / todo event system rely on.
package plugins

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writePlugin(t *testing.T, dir, name, body string) {
	t.Helper()
	sub := filepath.Join(dir, name)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "plugin.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPluginAgentConfigShape(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "hello-plugin", `name = "hello-plugin"
version = "1.0.0"
[[agents]]
name = "fastcoder"
type = "code"
model = "openai/gpt-4o"
provider = "openai"
system_file = "system.md"
`)

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatal(err)
	}
	cfgs := r.AgentConfigs()
	if len(cfgs) != 1 {
		t.Fatalf("expected 1 agent config, got %d", len(cfgs))
	}
	cfg := cfgs[0]

	// Name uses the canonical "plugin-<name>-<agent>" prefix.
	if cfg.Name != "plugin-hello-plugin-fastcoder" {
		t.Errorf("agent name: got %q, want plugin-hello-plugin-fastcoder", cfg.Name)
	}
	// Description is prefixed with [plugin X] so the orchestrator-agents
	// output can tag these distinctly from defaults.
	if !strings.Contains(cfg.Description, "[plugin hello-plugin]") {
		t.Errorf("description should be tagged with [plugin X]: got %q", cfg.Description)
	}
	// Fields are merged from the manifest verbatim.
	if cfg.Model != "openai/gpt-4o" {
		t.Errorf("model: got %q", cfg.Model)
	}
	if cfg.Provider != "openai" {
		t.Errorf("provider: got %q", cfg.Provider)
	}
	if !strings.HasSuffix(cfg.SystemFile, "system.md") {
		t.Errorf("system file should resolve under plugin dir, got %q", cfg.SystemFile)
	}
	if cfg.MemoryNS != "plugin-hello-plugin" {
		t.Errorf("memory namespace: got %q", cfg.MemoryNS)
	}
}

func TestPluginMCPToolName(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "hello-plugin", `name = "hello-plugin"
version = "1.0.0"
[[tools]]
name = "greet"
description = "Say hi to a user"
binary = "./bin/greet"
args = ["name", "shout"]
`)

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatal(err)
	}
	tools := r.MCPTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0]

	// The MCP tool name MUST be sin_plugin_<plugin>_<tool>.
	if tool.Name != "sin_plugin_hello-plugin_greet" {
		t.Errorf("MCP tool name: got %q, want sin_plugin_hello-plugin_greet", tool.Name)
	}
	// Description is prefixed with [plugin X].
	if !strings.HasPrefix(tool.Description, "[plugin hello-plugin]") {
		t.Errorf("description should start with [plugin X]: got %q", tool.Description)
	}
	// Args are converted to JSON Schema properties.
	props, ok := tool.Schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema.properties is not a map: %T", tool.Schema["properties"])
	}
	if _, ok := props["name"]; !ok {
		t.Errorf("expected 'name' in properties, got %v", props)
	}
	if _, ok := props["shout"]; !ok {
		t.Errorf("expected 'shout' in properties, got %v", props)
	}
	req, ok := tool.Schema["required"].([]string)
	if !ok || len(req) != 2 {
		t.Errorf("schema.required: got %v", tool.Schema["required"])
	}
}

func TestPluginHookFire(t *testing.T) {
	// Spawn a shell that writes the SIN_TODO_TITLE and SIN_TODO_ID env
	// vars to a tmpfile. Then check the tmpfile contents.
	dir := t.TempDir()
	marker := filepath.Join(dir, "fired.txt")

	// Build the manifest with a literal-string command (TOML basic
	// string, double-quoted) so $SIN_TODO_TITLE is preserved verbatim
	// for the shell to expand at runtime.
	manifest := fmt.Sprintf(`name = "hello-plugin"
version = "1.0.0"

[[hooks]]
event = "post_complete"
command = "printf 'X%%sY%%sZ' \"$SIN_TODO_TITLE\" \"$SIN_TODO_ID\" > %s"
`, marker)
	writePlugin(t, dir, "hello-plugin", manifest)

	r := NewRegistry()
	if err := r.LoadFromDir(dir); err != nil {
		t.Fatal(err)
	}
	hooks := r.HooksFor("post_complete")
	if len(hooks) != 1 {
		t.Fatalf("expected 1 post_complete hook, got %d", len(hooks))
	}
	if hooks[0].Plugin != "hello-plugin" {
		t.Errorf("hook plugin: got %q", hooks[0].Plugin)
	}
	if hooks[0].Event != "post_complete" {
		t.Errorf("hook event: got %q", hooks[0].Event)
	}

	// Run the hook as the wire-up code would: sh -c <cmd> with env.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", hooks[0].Command)
	c.Env = append(os.Environ(),
		"SIN_TODO_TITLE=Hello world",
		"SIN_TODO_ID=st-abc123",
		"SIN_EVENT=post_complete",
	)
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("hook exec failed: %v\n%s", err, string(out))
	}
	_ = out

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("marker not written: %v", err)
	}
	if want := "XHello worldYst-abc123Z"; string(got) != want {
		t.Errorf("marker contents: got %q, want %q", string(got), want)
	}
}

func TestPluginHooksForUnknownEvent(t *testing.T) {
	r := NewRegistry()
	if got := r.HooksFor("nonexistent_event"); len(got) != 0 {
		t.Errorf("expected 0 hooks for unknown event, got %d", len(got))
	}
}

func TestResolvePluginDirOverride(t *testing.T) {
	dir := t.TempDir()
	if got := ResolvePluginDir(dir); got != dir {
		t.Errorf("override: got %q want %q", got, dir)
	}
	prev, had := os.LookupEnv("SIN_CODE_CONFIG_DIR")
	defer func() {
		if had {
			os.Setenv("SIN_CODE_CONFIG_DIR", prev)
		} else {
			os.Unsetenv("SIN_CODE_CONFIG_DIR")
		}
	}()
	os.Setenv("SIN_CODE_CONFIG_DIR", dir)
	if got := ResolvePluginDir(""); got != filepath.Join(dir, "sin-code", "plugins") {
		t.Errorf("env: got %q want %q", got, filepath.Join(dir, "sin-code", "plugins"))
	}
}
